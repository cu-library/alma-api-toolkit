// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package api provides an HTTP client which works with the Alma API.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"context"
)

// TestDoAuthorizationKey checks that the Authorization header is set when making API calls.
func TestDoAuthorization(t *testing.T) {
	key := "a test key"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello!")
		if r.Header.Get("Authorization") != "apikey "+key {
			t.Fatal("unexpected Authorization header")
		}
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{
		Client: ts.Client(),
		Host:   tsURL.Host,
		Key:    key,
	}
	r, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := c.Do(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Hello!" {
		t.Fatal("unexpected bytes from Do()")
	}
}

// TestDoCancelledContext checks that the client returns an error if the context is already cancelled.
func TestDoCancelledContext(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello!")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{
		Client: ts.Client(),
		Host:   tsURL.Host,
	}
	r, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Do(ctx, r)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled context did not cause error")
	}
}

// TestThresholdReached checks that the client returns an error if the remaining api calls threshold has been reached.
func TestThresholdReached(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Exl-Api-Remaining", "1")
		fmt.Fprintf(w, "Hello!")
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{
		Client:    ts.Client(),
		Host:      tsURL.Host,
		Threshold: 2,
	}
	r, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), r)
	var over *ThresholdReachedError
	if !errors.As(err, &over) {
		t.Fatal("low remaining API calls did not cause error")
	}
}

// TestStatusCode checks that the client returns an error if the API returns an unexpected status code.
func TestStatusCode(t *testing.T) {
	t.Parallel()
	methods := []string{
		"GET",
		"HEAD",
		"POST",
		"PUT",
		"PATCH",
		"DELETE",
		"CONNECT",
		"OPTIONS",
		"TRACE",
	}
	statuses := []int{
		200,
		201,
		202,
		203,
		204,
		205,
		206,
		300,
		305,
		400,
		401,
		402,
		403,
		404,
		405,
		500,
		501,
		502,
		503,
		504,
	}
	for _, method := range methods {
		method := method
		for _, status := range statuses {
			status := status
			t.Run(method+" "+strconv.Itoa(status), func(t *testing.T) {
				t.Parallel() // marks each test case as capable of running in parallel with each other
				ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Hello!", status)
				}))
				defer ts.Close()
				tsURL, err := url.Parse(ts.URL)
				if err != nil {
					t.Fatal(err)
				}
				c := &Client{
					Client: ts.Client(),
					Host:   tsURL.Host,
				}
				r, err := http.NewRequest(method, "/", nil)
				if err != nil {
					t.Fatal(err)
				}
				_, err = c.Do(context.Background(), r)
				if err == nil {
					if method == "DELETE" && status == 204 {
						return
					}
					if status == 200 {
						return
					}
					t.Fatalf("HTTP method %v + status %v didn't return an error", method, status)
				}
			})
		}
	}
}
