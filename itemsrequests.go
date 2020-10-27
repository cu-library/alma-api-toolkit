// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

func (m SubcommandMap) addItemsRequests() {
	fs := flag.NewFlagSet("items-requests", flag.ExitOnError)
	setID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  View requests on items in the given set.")
		flagUsage(fs)
	}
	m[fs.Name()] = &Subcommand{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
		FlagSet:     fs,
		ValidateFlags: func() error {
			return validateSetFlags(*setID, *setName)
		},
		Run: func(requester Requester) []error {
			members, errs := getSetMembers(requester, *setID, *setName)
			if len(errs) != 0 {
				return errs
			}
			requests, errs := ViewRequests(requester, members)
			typeSubTypeCount := map[string]int{}
			for _, request := range requests {
				typeSubType := fmt.Sprintf("Type: %v Subtype: %v", request.RequestType, request.RequestSubType)
				typeSubTypeCount[typeSubType] = typeSubTypeCount[typeSubType] + 1
			}
			for typeSubType, count := range typeSubTypeCount {
				fmt.Println(typeSubType, "Count:", count)
			}
			return errs
		},
	}
}

// ViewRequests provides information about user requests
func ViewRequests(requester Requester, members []Member) (requests []UserRequest, errs []error) {
	errorsMux := sync.Mutex{}
	requestsMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	bar := defaultProgressBar(len(members))
	bar.Describe("Getting user requests")
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			memberRequests, err := viewRequests(requester, member)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				requestsMux.Lock()
				defer requestsMux.Unlock()
				requests = append(requests, memberRequests...)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	close(jobs)
	wg.Wait()
	return requests, errs
}

func viewRequests(requester Requester, member Member) (requests []UserRequest, err error) {
	url, err := url.Parse(member.Link + "/requests")
	if err != nil {
		return requests, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return requests, err
	}
	body, err := requester(r)
	if err != nil {
		return requests, err
	}
	returnedRequests := UserRequests{}
	err = xml.Unmarshal(body, &returnedRequests)
	if err != nil {
		return requests, fmt.Errorf("unmarshalling user requests XML failed: %w\n%v", err, string(body))
	}
	return returnedRequests.UserRequests, nil
}
