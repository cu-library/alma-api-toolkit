// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const requestTimeout = 30 * time.Second

// APIResponse stores information returned by the API.
type APIResponse struct {
	Body              []byte
	StatusCode        int
	RemainingAPICalls int
}

// CheckAPIandKey ensures the API is available and that the key provided has the right permissions. It returns the number of API calls remaining.
func CheckAPIandKey(ctx context.Context, client *http.Client, server, key string, readAccess, writeAccess []string) (int, error) {
	remainingAPICalls := 0

	type Check struct {
		Endpoint string
		Method   string
		Verb     string
	}
	checks := []Check{}

	for _, endpoint := range readAccess {
		checks = append(checks, Check{endpoint, "GET", "read"})
	}
	for _, endpoint := range writeAccess {
		checks = append(checks, Check{endpoint, "POST", "write"})
	}

	for _, check := range checks {
		resp, err := RequestWithBackoff(ctx, client, server, key, check.Method, check.Endpoint+"/test", nil)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != 200 {
			return resp.RemainingAPICalls, fmt.Errorf("%v on %v failed, HTTP Status Code %v\n%v", check.Verb, check.Endpoint, resp.StatusCode, string(resp.Body))
		}
		remainingAPICalls = resp.RemainingAPICalls
	}

	return remainingAPICalls, nil
}

// RequestWithBackoff makes HTTP requests until one is successful or a timeout is reached. It also drains and closes response bodies.
func RequestWithBackoff(baseCtx context.Context, client *http.Client, server, key string, method, path string, body io.Reader) (APIResponse, error) {
	// The initial and subsequent retries share a context with a timeout.
	ctx, cancel := context.WithTimeout(baseCtx, requestTimeout)
	defer cancel()

	// Build the request.
	u := &url.URL{
		Scheme: "https",
		Host:   server,
		Path:   path,
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return APIResponse{}, err
	}
	r.Header.Add("Authorization", "apikey "+key)

	// Retry requests on error with backoff.
	backoff := 0
	for {
		select {
		case <-ctx.Done():
			// The context is cancelled or timed out.
			return APIResponse{}, ctx.Err()
		case <-time.After(time.Duration(backoff) * time.Second):
			resp, err := client.Do(r)
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
				bodyBytes, err := ioutil.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return APIResponse{}, err
				}
				remainingCalls := resp.Header.Get("X-Exl-Api-Remaining")
				numRemaining, err := strconv.Atoi(remainingCalls)
				if err != nil {
					numRemaining = 0
				}
				return APIResponse{bodyBytes, resp.StatusCode, numRemaining}, nil
			}
		}
	}
}
