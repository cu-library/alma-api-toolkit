// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	limit          = 100
)

// Set stores data about sets.
type Set struct {
	XMLName         xml.Name `xml:"set"`
	ID              string   `xml:"id"`
	Name            string   `xml:"name"`
	NumberOfMembers int      `xml:"number_of_members"`
}

// Sets stores data about lists of sets.
type Sets struct {
	XMLName xml.Name `xml:"sets"`
	Sets    []Set    `xml:"set"`
}

// Members stores data about set members.
type Members struct {
	XMLName xml.Name `xml:"members"`
	Members []struct {
		ID string `xml:"id"`
	} `xml:"member"`
}

// CheckAPIandKey ensures the API is available and that the key provided has the right permissions.
func CheckAPIandKey(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key string, readAccess, writeAccess []string) error {
	type Check struct {
		Endpoint string
		Method   string
		Verb     string
	}
	checks := []Check{}
	for _, endpoint := range readAccess {
		checks = append(checks, Check{endpoint + "/test", "GET", "read"})
	}
	for _, endpoint := range writeAccess {
		checks = append(checks, Check{endpoint + "/test", "POST", "write"})
	}
	for _, check := range checks {
		url := &url.URL{
			Scheme: "https",
			Host:   server,
			Path:   check.Endpoint,
		}
		parsedURL := url.String()
		_, err := requestWithBackoff(ctx, client, remAPICalls, key, check.Method, parsedURL, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetSetIDFromName finds the set ID given a set name.
func GetSetIDFromName(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key, setName string) (string, error) {
	url := &url.URL{
		Scheme: "https",
		Host:   server,
		Path:   "/almaws/v1/conf/sets",
	}
	q := url.Query()
	q.Set("q", "name~"+setName)
	url.RawQuery = q.Encode()
	parsedURL := url.String()
	body, err := requestWithBackoff(ctx, client, remAPICalls, key, "GET", parsedURL, nil)
	if err != nil {
		return "", err
	}
	sets := Sets{}
	err = xml.Unmarshal(body, &sets)
	if err != nil {
		return "", fmt.Errorf("unmarshalling sets XML failed: %v\n%v", err, string(body))
	}
	for _, set := range sets.Sets {
		if strings.TrimSpace(set.Name) == strings.TrimSpace(setName) {
			return set.ID, nil
		}
	}
	return "", fmt.Errorf("no set with name %v found", setName)
}

// GetNumberOfMembers returns the number of members in a set.
func GetNumberOfMembers(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key, setID string) (int, error) {
	url := &url.URL{
		Scheme: "https",
		Host:   server,
		Path:   "/almaws/v1/conf/sets/" + setID,
	}
	parsedURL := url.String()
	body, err := requestWithBackoff(ctx, client, remAPICalls, key, "GET", parsedURL, nil)
	if err != nil {
		return 0, err
	}
	set := Set{}
	err = xml.Unmarshal(body, &set)
	if err != nil {
		return 0, fmt.Errorf("unmarshalling set XML failed: %v\n%v", err, string(body))
	}
	if set.NumberOfMembers == 0 {
		return 0, fmt.Errorf("set has no members or number of members not found")
	}
	return set.NumberOfMembers, nil
}

// GetMemberIDs returns the member IDs of a set.
func GetMemberIDs(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key, setID string, numMembers int) (allMemberIDs []string, err error) {
	membersSet := map[string]bool{}
	offsets := numMembers / limit
	// If there's a remainder, we need the extra offset.
	if numMembers%limit != 0 {
		offsets++
	}
	offsetsChan := make(chan int, offsets)
	memberIDsChan := make(chan []string, offsets)
	errorsChan := make(chan error, offsets)
	wg := sync.WaitGroup{}
	// Load the offsets channel.
	for i := 0; i < offsets; i++ {
		offsetsChan <- i * limit
	}
	close(offsetsChan)
	for worker := 0; worker < runtime.NumCPU(); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					errorsChan <- ctx.Err()
					return
				case offset, ok := <-offsetsChan:
					if !ok {
						return
					}
					memberIDs, err := getMemberIDForOffset(ctx, client, remAPICalls, server, key, setID, limit, offset)
					if err != nil {
						errorsChan <- err
						return
					}
					memberIDsChan <- memberIDs
				}
			}
		}()
	}
	wg.Wait()
	// Safe to close from receiving end because we know all senders are done.
	close(memberIDsChan)
	close(errorsChan)
	// If ok is true, we received an error from the channel. That means at least one error occured.
	err, ok := <-errorsChan
	if ok {
		return allMemberIDs, err
	}
	for memberIDs := range memberIDsChan {
		for _, memberID := range memberIDs {
			membersSet[memberID] = true
		}
	}
	if len(membersSet) != numMembers {
		return allMemberIDs, fmt.Errorf("%v members found for set with size %v", len(membersSet), numMembers)
	}
	allMemberIDs = make([]string, 0, len(membersSet)) // Small optimzation, we already know the size of the underlying array we need.
	for member := range membersSet {
		allMemberIDs = append(allMemberIDs, member)
	}
	return allMemberIDs, nil
}

func getMemberIDForOffset(ctx context.Context, client *http.Client, remAPICalls chan<- int, server, key, setID string, limit, offset int) (memberIDs []string, err error) {
	url := &url.URL{
		Scheme: "https",
		Host:   server,
		Path:   "/almaws/v1/conf/sets/" + setID + "/members",
	}
	q := url.Query()
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	url.RawQuery = q.Encode()
	parsedURL := url.String()
	body, err := requestWithBackoff(ctx, client, remAPICalls, key, "GET", parsedURL, nil)
	if err != nil {
		return memberIDs, err
	}
	members := Members{}
	err = xml.Unmarshal(body, &members)
	if err != nil {
		return memberIDs, fmt.Errorf("unmarshalling set XML failed: %v\n%v", err, string(body))
	}
	for _, member := range members.Members {
		memberIDs = append(memberIDs, member.ID)
	}
	return memberIDs, nil
}

// requestWithBackoff makes HTTP requests until one is successful or a timeout is reached. It also drains and closes response bodies.
func requestWithBackoff(baseCtx context.Context, client *http.Client, remAPICalls chan<- int, key string, method string, url string, requestBody io.Reader) (responseBody []byte, err error) {
	// The initial and subsequent retries share a context with a timeout.
	ctx, cancel := context.WithTimeout(baseCtx, requestTimeout)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return responseBody, err
	}
	r.Header.Add("Authorization", "apikey "+key)

	// Retry requests on error with backoff.
	backoff := 0
	for {
		select {
		// The context is cancelled or timed out.
		case <-ctx.Done():
			return responseBody, ctx.Err()
		case <-time.After(time.Duration(backoff) * time.Second):
			resp, err := client.Do(r)
			if errors.Is(err, context.Canceled) {
				return responseBody, err
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return responseBody, err
			}
			if err != nil {
				// If there is a response, drain and close the body.
				if resp != nil {
					_, _ = io.Copy(ioutil.Discard, resp.Body)
					_ = resp.Body.Close()
				}
				log.Printf("ERROR: Call to API failed, %v.\n", err)
				backoff++
				log.Printf("Retrying in %v seconds...\n", backoff)
			} else {
				responseBody, err := ioutil.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return responseBody, err
				}
				remainingCallsHeader := resp.Header.Get("X-Exl-Api-Remaining")
				numRemaining, err := strconv.Atoi(remainingCallsHeader)
				if err == nil {
					remAPICalls <- numRemaining
				}
				// The Alma API always returns a 200, even on successful POST requests. Treat any other status like an error.
				if resp.StatusCode != 200 {
					return responseBody, fmt.Errorf("%v on %v failed [%v]\n%v", method, url, resp.StatusCode, string(responseBody))
				}
				return responseBody, nil
			}
		}
	}
}
