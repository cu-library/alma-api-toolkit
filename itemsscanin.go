// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

func (m SubcommandMap) addItemsScanIn() {
	fs := flag.NewFlagSet("items-scan-in", flag.ExitOnError)
	setID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	circdesk := fs.String("circdesk", "", "The circ desk code.")
	library := fs.String("library", "", "The library code.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Scan items in.")
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
			if *circdesk == "" {
				return fmt.Errorf("a circ desk code is required")
			}
			if *library == "" {
				return fmt.Errorf("a library code is required")
			}
			return nil
		},
		Run: func(requester Requester) []error {
			members, errs := getSetMembers(requester, *setID, *setName)
			if len(errs) != 0 {
				return errs
			}
			count, errs := ScanIn(requester, members, *circdesk, *library)
			fmt.Printf("%v successful scans.\n", count)
			return errs
		},
	}
}

// ScanIn scans items in.
func ScanIn(requester Requester, members []Member, circdesk, library string) (count int, errs []error) {
	errorsMux := sync.Mutex{}
	countMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			err := scanIn(requester, member, circdesk, library)
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

func scanIn(requester Requester, member Member, circdesk, library string) error {
	url, err := url.Parse(member.Link)
	if err != nil {
		return err
	}
	q := url.Query()
	q.Set("op", "scan")
	q.Set("register_in_house_use", "false")
	q.Set("circ_desk", circdesk)
	q.Set("library", library)
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("POST", url.String(), nil)
	if err != nil {
		return err
	}
	_, err = requester(r)
	if err != nil {
		return err
	}
	return nil
}
