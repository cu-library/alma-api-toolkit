// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package cleanupcallnumbers provides a subcommand which cleans up call numbers in holdings records.
package cleanupcallnumbers

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/cu-library/almatoolkit/api"
	"github.com/cu-library/almatoolkit/subcommand"
)

// Config returns a new subcommand config.
func Config(envPrefix string) *subcommand.Config {
	fs := flag.NewFlagSet("bibs-clean-up-call-numbers", flag.ExitOnError)
	ID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	name := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	dryrun := fs.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	fs.Usage = func() {
		description := "Clean up the call numbers in the holdings records for a set of bib records.\n" +
			"\n" +
			"The following rules are run on the call numbers:\n" +
			"Add a space between a number then a letter.\n" +
			"Add a space between a number and a period when the period is followed by a letter.\n" +
			"Remove the extra periods from any substring matching space period period...\n" +
			"Remove any spaces between a period and a number.\n" +
			"Remove any leading or trailing whitespace."
		subcommand.Usage(fs, envPrefix, description)
	}
	return &subcommand.Config{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
		FlagSet:     fs,
		ValidateFlags: func() error {
			return subcommand.ValidateSetNameAndSetIDFlags(*name, *ID)
		},
		Run: func(ctx context.Context, c *api.Client) error {
			if *dryrun {
				log.Println("Running in dry run mode, no changes will be made in Alma.")
			} else {
				log.Println("WARNING: Not running in dry run mode, changes will be made in Alma!")
			}
			set, err := c.SetFromNameOrID(ctx, *name, *ID)
			if err != nil {
				return err
			}
			if set.Type != "ITEMIZED" || set.Content != "BIB_MMS" {
				return fmt.Errorf("the set must be an itemized set of bibs")
			}
			members, errs := c.SetMembers(ctx, set)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving the members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			holdingListMembers, errs := c.BibMembersHoldingListMembers(ctx, members)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving the holding list members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			holdings, errs := c.HoldingListMembersToHoldings(ctx, holdingListMembers)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving the holdings records of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			cleaned := CleanUpCallNumbers(holdings)
			cleanedMap := map[string]api.Holding{}
			for _, holding := range cleaned {
				cleanedMap[holding.HoldingListMember.Link] = holding
			}
			updated := []api.Holding{}
			errs = []error{}
			if !*dryrun {
				updated, errs = c.HoldingsUpdate(ctx, cleaned)
			}
			updatedMap := map[string]bool{}
			for _, holding := range updated {
				updatedMap[holding.HoldingListMember.Link] = true
			}
			w := csv.NewWriter(os.Stdout)
			err = w.Write([]string{"Link", "Original call number", "Updated call number", "Changed in Alma"})
			if err != nil {
				return fmt.Errorf("error writing csv header: %w", err)
			}
			for _, holding := range holdings {
				line := []string{holding.HoldingListMember.Link, holding.EightFiftyTwoSubHSubI()}
				cleanedHolding, inCleaned := cleanedMap[holding.HoldingListMember.Link]
				if inCleaned {
					line = append(line, cleanedHolding.EightFiftyTwoSubHSubI())
				} else {
					line = append(line, "")
				}
				_, inUpdated := updatedMap[holding.HoldingListMember.Link]
				if inUpdated {
					line = append(line, "yes")
				} else {
					line = append(line, "no")
				}
				err := w.Write(line)
				if err != nil {
					return fmt.Errorf("error writing line to csv: %w", err)
				}
			}
			w.Flush()
			err = w.Error()
			if err != nil {
				return fmt.Errorf("error writing line to csv: %w", err)
			}
			log.Printf("%v successful updates to call numbers.\n", len(updated))
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when updating the call numbers of holdings records of bibs in '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			return nil
		},
	}
}

// CleanUpCallNumbers cleans up the call numbers in the holdings records.
func CleanUpCallNumbers(holdings []api.Holding) (cleaned []api.Holding) {
	bar := api.DefaultProgressBar(len(holdings))
	bar.Describe("Cleaning call numbers")
	for _, holding := range holdings {
		updated := false
		for fi, field := range holding.Record.Datafield {
			if field.Tag == "852" {
				for si, sub := range field.Subfield {
					if sub.Code == "h" || sub.Code == "i" {
						updatedCallNum := CleanupCallNumberSubfield(sub.Text)
						if updatedCallNum != sub.Text {
							holding.Record.Datafield[fi].Subfield[si].Text = updatedCallNum
							updated = true
						}
					}
				}
			}
		}
		if updated {
			cleaned = append(cleaned, holding)
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	return cleaned
}

// CleanupCallNumberSubfield returns a call number which is cleaned up.
func CleanupCallNumberSubfield(callNum string) string {
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
