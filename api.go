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

// HoldingListMember stores data about a holding record when it's returned from the Holdings list
// /almaws/v1/bibs/{mms_id}/holdings
type HoldingListMember struct {
	Link      string `xml:"link,attr"`
	HoldingID string `xml:"holding_id"`
	Library   struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"library"`
	Location struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"location"`
	CallNumber             string `xml:"call_number"`
	SuppressFromPublishing string `xml:"suppress_from_publishing"`
}

// Holdings stores data about holdings under a bib record.
type Holdings struct {
	XMLName          xml.Name            `xml:"holdings"`
	TotalRecordCount string              `xml:"total_record_count,attr"`
	HoldingsRecords  []HoldingListMember `xml:"holding"`
	BibData          struct {
		Link           string `xml:"link,attr"`
		MmsID          string `xml:"mms_id"`
		Title          string `xml:"title"`
		ISSN           string `xml:"issn"`
		NetworkNumbers struct {
			Text          string `xml:",chardata"`
			NetworkNumber string `xml:"network_number"`
		} `xml:"network_numbers"`
		Publisher string `xml:"publisher"`
	} `xml:"bib_data"`
}

// Holding stores data about a holding record.
// /almaws/v1/bibs/{mms_id}/holdings/{holding_id}
type Holding struct {
	XMLName                xml.Name `xml:"holding"`
	HoldingID              string   `xml:"holding_id"`
	CreatedBy              string   `xml:"created_by"`
	CreatedDate            string   `xml:"created_date"`
	OriginatingSystem      string   `xml:"originating_system"`
	OriginatingSystemID    string   `xml:"originating_system_id"`
	SuppressFromPublishing string   `xml:"suppress_from_publishing"`
	Record                 struct {
		Text         string `xml:",chardata"`
		Leader       string `xml:"leader"`
		Controlfield []struct {
			Text string `xml:",chardata"`
			Tag  string `xml:"tag,attr"`
		} `xml:"controlfield"`
		Datafield []struct {
			Text     string `xml:",chardata"`
			Ind1     string `xml:"ind1,attr"`
			Ind2     string `xml:"ind2,attr"`
			Tag      string `xml:"tag,attr"`
			Subfield []struct {
				Text string `xml:",chardata"`
				Code string `xml:"code,attr"`
			} `xml:"subfield"`
		} `xml:"datafield"`
	} `xml:"record"`
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
func MakeRequestFunc(ctx context.Context, cancel context.CancelFunc, client *http.Client, remainingAPICallsThreshold int, server, key string) func(*http.Request) ([]byte, error) {
	return func(r *http.Request) ([]byte, error) {
		return requestWithBackoff(ctx, cancel, client, remainingAPICallsThreshold, server, key, r)
	}
}

// requestWithBackoff makes HTTP requests until one is successful or a timeout is reached.
// This function closes and drains the response bodies. https://golang.org/pkg/net/http/#Client.Do
func requestWithBackoff(baseCtx context.Context, baseCancel context.CancelFunc, client *http.Client, remainingAPICallsThreshold int, server, key string, r *http.Request) (responseBody []byte, err error) {
	// The initial and subsequent retries share a context with a timeout.
	ctx, cancel := context.WithTimeout(baseCtx, requestTimeout)
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
			// If the number of remaining API calls is below the threshold,
			// call the parent cancel function.
			remainingCallsHeader := resp.Header.Get("X-Exl-Api-Remaining")
			remainingAPICalls, err := strconv.Atoi(remainingCallsHeader)
			if err == nil && remainingAPICalls <= remainingAPICallsThreshold {
				log.Printf("FATAL: API call threshold of %v reached, only %v calls remaining.\n", remainingAPICallsThreshold, remainingAPICalls)
				baseCancel()
			}
			// The Alma API always returns a 200 status on success, except for a successful DELETE, which returns 204.
			if (r.Method == "DELETE" && resp.StatusCode != 204) || (r.Method != "DELETE" && resp.StatusCode != 200) {
				return responseBody, fmt.Errorf("%v on %v failed [%v]\n%v", r.Method, r.URL.String(), resp.StatusCode, string(responseBody))
			}
			return responseBody, nil
		}
	}
}
