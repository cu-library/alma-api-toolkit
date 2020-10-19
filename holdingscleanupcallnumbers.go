// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
)

func (m SubcommandMap) addHoldingsCleanUpCallNumbers() {
	fs := flag.NewFlagSet("holdings-clean-up-call-numbers", flag.ExitOnError)
	setID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	dryrun := fs.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Clean up the call numbers on a set of holdings records.")
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
			count, errs := CleanUpCallNumbers(requester, members, *dryrun)
			fmt.Printf("%v call numbers processed.\n", count)
			return errs
		},
	}
}

// CleanUpCallNumbers cleans up call numbers in holdings records.
func CleanUpCallNumbers(requester Requester, members []Member, dryrun bool) (count int, errs []error) {
	errorsMux := sync.Mutex{}
	countMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			err := cleanUpCallNumbers(requester, member, dryrun)
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
	}
	close(jobs)
	wg.Wait()
	return count, errs
}

func cleanUpCallNumbers(requester Requester, member Member, dryrun bool) error {
	url, err := url.Parse(member.Link + "/holdings")
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
	log.Println(dryrun)
	log.Println(string(body))
	return nil
}
