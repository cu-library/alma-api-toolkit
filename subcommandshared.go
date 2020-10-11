// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

func subcommandEnvPrefix(prefix, name string) string {
	return prefix + strings.ToUpper(strings.ReplaceAll(name, "-", "")) + "_"
}

func flagUsage(fs *flag.FlagSet) {
	fs.PrintDefaults()
	fmt.Fprintln(flag.CommandLine.Output(), "  Environment variables read when flag is unset:")
	fs.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(flag.CommandLine.Output(), "  %v%v\n", subcommandEnvPrefix(EnvPrefix, fs.Name()), strings.ToUpper(f.Name))
	})
}

func validateSetFlags(setName, setID string) error {
	if setName == "" && setID == "" {
		return fmt.Errorf("a set name or a set ID are required")
	}
	if setName != "" && setID != "" {
		return fmt.Errorf("a set name OR a set ID can be provided, not both")
	}
	return nil
}

func getSetMembers(requester Requester, setID, setName string) (members []Member, errs []error) {
	if setName != "" {
		set, err := GetSetFromName(requester, setName)
		if err != nil {
			return members, append(errs, fmt.Errorf("getting set ID from name failed: %w", err))
		}
		log.Printf("ID '%v' found for set name '%v'.", set.ID, setName)
		setID = set.ID
	}
	set, err := GetSetFromID(requester, setID)
	if err != nil {
		return members, append(errs, fmt.Errorf("getting set from ID failed: %w", err))
	}
	return GetMembers(requester, set)
}

// GetSetFromName returns the set with the given set name.
func GetSetFromName(requester Requester, setName string) (set Set, err error) {
	url := &url.URL{
		Path: "/almaws/v1/conf/sets",
	}
	q := url.Query()
	q.Set("q", "name~"+setName)
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return set, err
	}
	body, err := requester(r)
	if err != nil {
		return set, err
	}
	sets := Sets{}
	err = xml.Unmarshal(body, &sets)
	if err != nil {
		return set, fmt.Errorf("unmarshalling sets XML failed: %w\n%v", err, string(body))
	}
	for _, set := range sets.Sets {
		if strings.TrimSpace(set.Name) == strings.TrimSpace(setName) {
			return set, nil
		}
	}
	return set, fmt.Errorf("no set with name %v found", setName)
}

// GetSetFromID returns the set with the given set ID.
func GetSetFromID(requester Requester, setID string) (set Set, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/sets/"+setID, nil)
	if err != nil {
		return set, err
	}
	body, err := requester(r)
	if err != nil {
		return set, err
	}
	err = xml.Unmarshal(body, &set)
	if err != nil {
		return set, fmt.Errorf("unmarshalling set XML failed: %w\n%v", err, string(body))
	}
	return set, nil
}

// GetMembers returns the members of a set.
func GetMembers(requester Requester, set Set) (members []Member, errs []error) {
	errorsMux := sync.Mutex{}
	membersMux := sync.Mutex{}
	membersSet := map[string]Member{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	offsets := set.NumberOfMembers / limit
	// If there's a remainder, we need the extra offset.
	if set.NumberOfMembers%limit != 0 {
		offsets++
	}
	for i := 0; i < offsets; i++ {
		offset := i * limit
		jobs <- func() {
			members, err := getMembers(requester, set, limit, offset)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				membersMux.Lock()
				defer membersMux.Unlock()
				for _, member := range members {
					membersSet[member.ID] = member
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	if len(membersSet) != set.NumberOfMembers {
		return members, append(errs, fmt.Errorf("%v members found for set with size %v", len(membersSet), set.NumberOfMembers))
	}
	members = make([]Member, 0, len(membersSet)) // Small optimzation, we already know the size of the underlying array we need.
	for _, member := range membersSet {
		members = append(members, member)
	}
	return members, errs
}

func getMembers(requester Requester, set Set, limit, offset int) (members []Member, err error) {
	url, err := url.Parse(set.Link + "/members")
	if err != nil {
		return members, err
	}
	q := url.Query()
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return members, err
	}
	body, err := requester(r)
	if err != nil {
		return members, err
	}
	returnedMembers := Members{}
	err = xml.Unmarshal(body, &returnedMembers)
	if err != nil {
		return members, fmt.Errorf("unmarshalling set XML failed: %w\n%v", err, string(body))
	}
	return returnedMembers.Members, nil
}
