// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package api provides an HTTP client which works with the Alma API.
package api

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	// RequestTimeout is the amount of time the tool will wait for API calls to complete before they are cancelled.
	RequestTimeout = 30 * time.Second

	// LimitParam is the limit parameter to offset+limit calls.
	LimitParam = 100

	// DefaultThreshold is the minimum number of API calls remaining before the tool automatically stops working.
	DefaultThreshold = 50000

	// DefaultAlmaAPIHost is the default Alma API Server domain name.
	DefaultAlmaAPIHost = "api-ca.hosted.exlibrisgroup.com"
)

// Client is a custom HTTP client for the Alma API.
type Client struct {
	// Client is the embedded http client.
	*http.Client
	// Host is the host name (domain name) for the Alma API we are calling.
	Host string
	// Key is the authorization key to use when calling the Alma API.
	Key string
	// Threshold is the minimum number of API calls remaining before
	// the Cancel function is called.
	Threshold int
}

// ThresholdReachedError is an error returned when the API remaining call limit has been reached.
type ThresholdReachedError struct {
	// Remaining is the number of calls remaining.
	Remaining int
	// Threshold is the call number minimum threshold.
	Threshold int
}

func (e ThresholdReachedError) Error() string {
	return fmt.Sprintf("call threshold of %v reached, %v calls remaining", e.Threshold, e.Remaining)
}

// CheckAPIandKey ensures the API is available and that the key provided has the right permissions.
func (c *Client) CheckAPIandKey(ctx context.Context, readAccess, writeAccess []string) error {
	for _, endpoint := range readAccess {
		r, err := http.NewRequest("GET", endpoint+"/test", nil)
		if err != nil {
			return err
		}
		_, err = c.Do(ctx, r)
		if err != nil {
			return err
		}
	}
	for _, endpoint := range writeAccess {
		r, err := http.NewRequest("POST", endpoint+"/test", nil)
		if err != nil {
			return err
		}
		_, err = c.Do(ctx, r)
		if err != nil {
			return err
		}
	}
	return nil
}

// Do makes HTTP requests with the Client to the Host using the Key.
// If a request returns an error, it is retried until RequestTimeout is reached.
// The response bodies are copied or drained, then closed.
// See https://golang.org/pkg/net/http/#Client.Do
func (c *Client) Do(ctx context.Context, r *http.Request) (body []byte, err error) {
	// Create a new context with a timeout so we don't retry forever.
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	// The scheme should always be https.
	r.URL.Scheme = "https"
	// The host should always be the client's Host.
	r.URL.Host = c.Host
	// Create a new request with the new context.
	r = r.WithContext(ctx)
	// Add the api key authorization header. https://developers.exlibrisgroup.com/alma/apis/#calling
	r.Header.Add("Authorization", "apikey "+c.Key)
	// Retry the request in a loop.
	backoff := 0
	for {
		select {
		// The context is cancelled or timed out.
		case <-ctx.Done():
			return body, fmt.Errorf("%v %v: %w", r.Method, r.URL.String(), ctx.Err())
		// We've waited backoff seconds, send the request.
		case <-time.After(time.Duration(backoff) * time.Second):
			// The select statement chooses one case at random if multiple are ready.
			// It is therefore possible that the context is cancelled.
			if ctx.Err() != nil {
				return body, fmt.Errorf("%v %v: %w", r.Method, r.URL.String(), ctx.Err())
			}
			// Make the request using the embedded http.Client.
			resp, err := c.Client.Do(r)
			// "An error is returned if caused by client policy (such as CheckRedirect),
			//  or failure to speak HTTP (such as a network connectivity problem).
			//  A non-2xx status code doesn't cause an error."
			// Check if the error is caused by a cancelled or expired context.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return body, err
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
			//  If the Body is not both read to EOF and closed, the Client's underlying RoundTripper (typically Transport)
			//  may not be able to re-use a persistent TCP connection to the server for a subsequent "keep-alive" request."
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				_ = resp.Body.Close()
				return body, err
			}
			err = resp.Body.Close()
			if err != nil {
				return body, err
			}
			// If the number of remaining API calls is below the threshold,
			// return a custom error called ThresholdReachedError, which can be checked later using
			// errors.As().
			rem, err := strconv.Atoi(resp.Header.Get("X-Exl-Api-Remaining"))
			if err == nil && rem <= c.Threshold {
				return body, &ThresholdReachedError{rem, c.Threshold}

			}
			// The Alma API always returns a 200 status on success, except for a successful DELETE, which returns 204.
			if (r.Method == "DELETE" && resp.StatusCode != 204) || (r.Method != "DELETE" && resp.StatusCode != 200) {
				return body, fmt.Errorf("%v %v failed [%v]\n%v", r.Method, r.URL.String(), resp.StatusCode, string(body))
			}
			return body, nil
		}
	}
}

// StartWorkers starts NumCPU workers in a worker pool which run jobs from the jobs channel until it is closed.
func StartWorkers(wg *sync.WaitGroup, jobs <-chan func()) {
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

// StartConcurrent initializes the context, mutexes, job channel, wait group, and progress bar for concurrent job processing.
func StartConcurrent(ctx context.Context, numJobs int, desc string) (context.Context, context.CancelFunc, *sync.Mutex, *sync.Mutex, chan<- func(), *sync.WaitGroup, *progressbar.ProgressBar) {
	ctx, cancel := context.WithCancel(ctx)
	// Errors Mux
	em := &sync.Mutex{}
	// Output Mux
	om := &sync.Mutex{}
	jobs := make(chan func())
	wg := &sync.WaitGroup{}
	StartWorkers(wg, jobs)
	bar := DefaultProgressBar(numJobs)
	bar.Describe("Getting code tables")
	return ctx, cancel, em, om, jobs, wg, bar
}

// StopConcurrent closes the jobs channel and waits for processing to end.
func StopConcurrent(jobs chan<- func(), wg *sync.WaitGroup) {
	close(jobs)
	wg.Wait()
}

// DefaultProgressBar returns a progress bar with common options already set.
func DefaultProgressBar(max int) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(
		max,
		progressbar.OptionSetWriter(log.Writer()),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
	)
	_ = bar.RenderBlank()
	return bar
}
