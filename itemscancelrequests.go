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

func (m SubcommandMap) addItemsCancelRequests() {
	fs := flag.NewFlagSet("items-cancel-requests", flag.ExitOnError)
	setID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	requestType := fs.String("type", "", "The request type to cancel. ex: WORK_ORDER")
	requestSubType := fs.String("subtype", "", "The request subtype to cancel.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Cancel item requests of type and/or subtype on items in the given set.")
		flagUsage(fs)
	}
	m[fs.Name()] = &Subcommand{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
		FlagSet:     fs,
		ValidateFlags: func() error {
			err := validateSetFlags(*setID, *setName)
			if err != nil {
				return err
			}
			if *requestType == "" && *requestSubType == "" {
				return fmt.Errorf("a request type or a request sub type are required")
			}
			return nil
		},
		Run: func(requester Requester) []error {
			members, errs := getSetMembers(requester, *setID, *setName)
			if len(errs) != 0 {
				return errs
			}
			count, errs := CancelRequests(requester, members, *requestType, *requestSubType)
			fmt.Printf("%v requests cancelled.\n", count)
			return errs
		},
	}
}

// CancelRequests cancels requests on an item.
func CancelRequests(requester Requester, members []Member, requestType, requestSubType string) (count int, errs []error) {
	errorsMux := sync.Mutex{}
	countMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	bar := defaultProgressBar(len(members))
	bar.Describe("Cancelling user requests")
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			err := cancelRequests(requester, member, requestType, requestSubType)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				countMux.Lock()
				defer countMux.Unlock()
				count++
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	close(jobs)
	wg.Wait()
	return count, errs
}

func cancelRequests(requester Requester, member Member, requestType, requestSubType string) error {
	url, err := url.Parse(member.Link + "/requests")
	if err != nil {
		return err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	body, err := requester(r)
	if err != nil {
		return err
	}
	requests := UserRequests{}
	err = xml.Unmarshal(body, &requests)
	if err != nil {
		return fmt.Errorf("unmarshalling user requests XML failed: %w\n%v", err, string(body))
	}
	for _, request := range requests.UserRequests {
		if request.MatchTypeSubType(requestType, requestSubType) {
			url, err := url.Parse(member.Link + "/requests/" + request.RequestID)
			if err != nil {
				return err
			}
			r, err := http.NewRequest("DELETE", url.String(), nil)
			if err != nil {
				return err
			}
			_, err = requester(r)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
