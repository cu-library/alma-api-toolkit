// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	limit          = 100 // The limit parameter to offset+limit calls.
)

// Set stores data about sets.
type Set struct {
	ID              string `xml:"id"`
	Name            string `xml:"name"`
	NumberOfMembers int    `xml:"number_of_members"`
	Link            string `xml:"link,attr"`
}

// Sets stores data about lists of sets as returned from the API.
// This is a little different than []Set for XML unmarshalling.
type Sets struct {
	XMLName xml.Name `xml:"sets"`
	Sets    []Set    `xml:"set"`
}

// Member stores data about members of sets.
type Member struct {
	ID   string `xml:"id"`
	Link string `xml:"link,attr"`
}

// Members stores data about set members.
// This is a little different than []Member for XML unmarshalling.
type Members struct {
	XMLName xml.Name `xml:"members"`
	Members []Member `xml:"member"`
}

// UserRequest stores data about a user request on items.
type UserRequest struct {
	RequestID      string `xml:"request_id"`
	RequestType    string `xml:"request_type"`
	RequestSubType string `xml:"request_sub_type"`
}

// MatchTypeSubType returns true if requestType is empty or matches the RequestType
// and the requestSubType is empty or matches the RequestSubType.
func (u UserRequest) MatchTypeSubType(requestType, requestSubType string) bool {
	if (requestType == "" || requestType == u.RequestType) && (requestSubType == "" || requestSubType == u.RequestSubType) {
		return true
	}
	return false
}

// UserRequests stores data about user requests on an item.
// This is a little different than []UserRequest for XML unmarshalling.
type UserRequests struct {
	XMLName      xml.Name      `xml:"user_requests"`
	UserRequests []UserRequest `xml:"user_request"`
}

// CheckAPIandKey ensures the API is available and that the key provided has the right permissions.
func CheckAPIandKey(requester Requester, readAccess, writeAccess []string) error {
	for _, endpoint := range readAccess {
		r, err := http.NewRequest("GET", endpoint+"/test", nil)
		if err != nil {
			return err
		}
		_, err = requester(r)
		if err != nil {
			return err
		}
	}
	for _, endpoint := range writeAccess {
		r, err := http.NewRequest("POST", endpoint+"/test", nil)
		if err != nil {
			return err
		}
		_, err = requester(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func startWorkers(wg *sync.WaitGroup, jobs <-chan func()) {
	for worker := 0; worker < runtime.NumCPU(); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				job()
			}
		}()
	}
}

// MakeRequestFunc returns a closure with configured parameters already set.
func MakeRequestFunc(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key string) func(*http.Request) ([]byte, error) {
	return func(r *http.Request) ([]byte, error) {
		return requestWithBackoff(ctx, client, remAPICalls, server, key, r)
	}
}

// requestWithBackoff makes HTTP requests until one is successful or a timeout is reached.
// The number of remaining API calls is sent on the remAPICalls channel.
// This function closes and drains the response bodies. https://golang.org/pkg/net/http/#Client.Do
func requestWithBackoff(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key string, r *http.Request) (responseBody []byte, err error) {
	// The initial and subsequent retries share a context with a timeout.
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	// The sheme should always be https.
	r.URL.Scheme = "https"

	// If the host isn't set, set it to the Alma API server.
	if r.URL.Host == "" {
		r.URL.Host = server
	}

	// Create a new request with the new context.
	r = r.WithContext(ctx)
	// Add the api key authorization header. https://developers.exlibrisgroup.com/alma/apis/#calling
	r.Header.Add("Authorization", "apikey "+key)

	// Retry requests on error with backoff.
	backoff := 0
	for {
		select {
		// The context is cancelled or timed out.
		case <-ctx.Done():
			return responseBody, fmt.Errorf("%v \"%v\": %w", strings.Title(strings.ToLower(r.Method)), r.URL.String(), ctx.Err())
		// We've waited backoff seconds, send the request.
		case <-time.After(time.Duration(backoff) * time.Second):
			// Make the request using the client.
			resp, err := client.Do(r)
			// "An error is returned if caused by client policy (such as CheckRedirect), or failure to speak HTTP (such as a network connectivity problem).
			//  A non-2xx status code doesn't cause an error."
			// Check if the error is caused by a cancelled or expired context.
			if errors.Is(err, context.Canceled) {
				return responseBody, err
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return responseBody, err
			}
			// The error was likely caused by a failure to connect to the server. Retry with linear backoff.
			if err != nil {
				// "On error, any Response can be ignored." No need to drain and close the body.
				// Log is safe to use concurrently.
				log.Printf("ERROR: Call to API failed, %v.\n", err)
				backoff++
				log.Printf("Retrying in %v seconds...\n", backoff)
				// Loop again.
				continue
			}
			// The error was nil.
			// "If the returned error is nil, the Response will contain a non-nil Body which the user is expected to close.
			//  If the Body is not both read to EOF and closed, the Client's underlying RoundTripper (typically Transport) may not be able
			//  to re-use a persistent TCP connection to the server for a subsequent "keep-alive" request."
			responseBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				_ = resp.Body.Close()
				return responseBody, err
			}
			err = resp.Body.Close()
			if err != nil {
				return responseBody, err
			}
			remainingCallsHeader := resp.Header.Get("X-Exl-Api-Remaining")
			numRemaining, err := strconv.Atoi(remainingCallsHeader)
			if err == nil {
				// We try our best, but if the API returns something invalid in the X-Exl-Api-Remaining header, we continue on.
				remAPICalls <- numRemaining
			}
			// The Alma API always returns a 200 status on success, except for a successful DELETE, which returns 204.
			if (r.Method == "DELETE" && resp.StatusCode != 204) || (r.Method != "DELETE" && resp.StatusCode != 200) {
				return responseBody, fmt.Errorf("%v on %v failed [%v]\n%v", r.Method, r.URL.String(), resp.StatusCode, string(responseBody))
			}
			return responseBody, nil
		}
	}
}
