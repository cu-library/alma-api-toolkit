// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"encoding/csv"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
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
			log.Println(len(members), "members found.")
			holdingRecords, errs := GetHoldingsRecords(requester, members)
			if len(errs) != 0 {
				return errs
			}
			log.Println(len(holdingRecords), "holding records found.")
			output, errs := CleanUpCallNumbers(requester, holdingRecords, *dryrun)
			log.Printf("%v call numbers processed.\n", len(output))

			w := csv.NewWriter(os.Stdout)
			err := w.Write([]string{"link", "original call number", "updated call number"})
			if err != nil {
				errs = append(errs, fmt.Errorf("error writing csv header: %w", err))
				return errs
			}

			for _, line := range output {
				err := w.Write(line)
				if errs != nil {
					errs = append(errs, fmt.Errorf("error writing line to csv: %w", err))
					return errs
				}
			}

			w.Flush()
			err = w.Error()
			if err != nil {
				errs = append(errs, fmt.Errorf("error after flushing csv: %w", err))
				return errs
			}

			return errs
		},
	}
}

// GetHoldingsRecords returns the holdings records for the members of the set.
func GetHoldingsRecords(requester Requester, members []Member) (holdingRecords []HoldingListMember, errs []error) {
	errorsMux := sync.Mutex{}
	recordsMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			memberHoldings, err := getHoldingsRecords(requester, member)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				recordsMux.Lock()
				defer recordsMux.Unlock()
				holdingRecords = append(holdingRecords, memberHoldings...)
			}
		}
	}
	close(jobs)
	wg.Wait()
	return holdingRecords, errs
}

func getHoldingsRecords(requester Requester, member Member) (holdingRecords []HoldingListMember, err error) {
	url, err := url.Parse(member.Link + "/holdings")
	if err != nil {
		return holdingRecords, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return holdingRecords, err
	}
	body, err := requester(r)
	if err != nil {
		return holdingRecords, err
	}
	holdings := Holdings{}
	err = xml.Unmarshal(body, &holdings)
	if err != nil {
		return holdingRecords, fmt.Errorf("unmarshalling holdings XML failed: %w\n%v", err, string(body))
	}
	return holdings.HoldingsRecords, nil
}

// CleanUpCallNumbers cleans up the call numbers in the holdings records.
func CleanUpCallNumbers(requester Requester, holdingRecords []HoldingListMember, dryrun bool) (output [][]string, errs []error) {
	errorsMux := sync.Mutex{}
	outputMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)

	for _, record := range holdingRecords {
		record := record // avoid closure refering to wrong value
		jobs <- func() {
			outputLines, err := cleanUpCallNumbers(requester, record, dryrun)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				outputMux.Lock()
				defer outputMux.Unlock()
				output = append(output, outputLines...)
			}
		}
	}
	close(jobs)
	wg.Wait()

	return output, errs
}

func cleanUpCallNumbers(requester Requester, holdingRecord HoldingListMember, dryrun bool) (output [][]string, err error) {
	url, err := url.Parse(holdingRecord.Link)
	if err != nil {
		return output, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return output, err
	}
	body, err := requester(r)
	if err != nil {
		return output, err
	}
	holding := Holding{}
	err = xml.Unmarshal(body, &holding)
	if err != nil {
		return output, fmt.Errorf("unmarshalling holding XML failed: %w\n%v", err, string(body))
	}

	//updated := false
	for fi, field := range holding.Record.Datafield {
		if field.Tag == "852" {
			for si, sub := range field.Subfield {
				if sub.Code == "h" || sub.Code == "i" {
					updatedCallNum := cleanupCallNumberSubfield(sub.Text)
					if updatedCallNum != sub.Text {
						output = append(output, []string{holdingRecord.Link, sub.Text, updatedCallNum})
						holding.Record.Datafield[fi].Subfield[si].Text = updatedCallNum
						//updated = true
					}
				}
			}
		}
	}

	return output, nil
}

func cleanupCallNumberSubfield(callNum string) string {
	// Add a space between a number then a letter.
	re := regexp.MustCompile(`([0-9])([a-zA-Z])`)
	callNum = re.ReplaceAllString(callNum, "$1 $2")
	// Add a space between a number and a period when the period is followed by a letter.
	re = regexp.MustCompile(`([0-9])\.([a-zA-Z])`)
	callNum = re.ReplaceAllString(callNum, "$1 .$2")
	// Remove the extra periods from any substring matching space period period...
	re = regexp.MustCompile(` \.\.+'`)
	callNum = re.ReplaceAllString(callNum, " .")
	// Remove any spaces between a period and a number.
	re = regexp.MustCompile(`\. +([0-9])`)
	callNum = re.ReplaceAllString(callNum, ".$1")
	// Remove any leading or trailing whitespace.
	callNum = strings.TrimSpace(callNum)
	return callNum
}
