// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package cancelrequests provides a subcommand which cancels the requests on items in a set.
package cancelrequests

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cu-library/almatoolkit/api"
	"github.com/cu-library/almatoolkit/subcommand"
)

// Config returns a new subcommand config.
func Config() *subcommand.Config {
	fs := flag.NewFlagSet("items-cancel-requests", flag.ExitOnError)
	ID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	name := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	rType := fs.String("type", "", "The request type to cancel. ex: WORK_ORDER")
	subType := fs.String("subtype", "", "The request subtype to cancel.")
	dryrun := fs.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Cancel item requests of type and/or subtype on items in the given set.")
	}
	return &subcommand.Config{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
		FlagSet:     fs,
		ValidateFlags: func() error {
			err := subcommand.ValidateSetNameAndSetIDFlags(*name, *ID)
			if err != nil {
				return err
			}
			if *rType == "" && *subType == "" {
				return fmt.Errorf("a request type or a request sub type are required")
			}
			return nil
		},
		Run: func(ctx context.Context, c *api.Client) error {
			if *dryrun {
				log.Println("Running in dry run mode, no changes will be made in Alma.")
			} else {
				log.Println("WARNING: Not running in dry run mode, changes will be made in Alma!")
			}
			set, err := c.SetFromNameOrID(ctx, *ID, *name)
			if err != nil {
				return err
			}
			if set.Type != "LOGICAL" || set.Content != "ITEM" {
				return fmt.Errorf("the set must be a logical set of items")
			}
			members, errs := c.SetMembers(ctx, set)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving the members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			requests, errs := c.ItemMembersUserRequests(ctx, members)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving requests on members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			matching := []api.UserRequest{}
			for _, request := range requests {
				if request.MatchTypeSubType(*rType, *subType) {
					matching = append(matching, request)
				}
			}
			matchingMap := map[string]bool{}
			for _, request := range matching {
				matchingMap[request.Member.Link] = true
			}
			cancelled := []api.UserRequest{}
			errs = []error{}
			if !*dryrun {
				cancelled, errs = c.UserRequestsCancel(ctx, matching)
			}
			cancelledMap := map[string]bool{}
			for _, request := range cancelled {
				cancelledMap[request.Member.Link] = true
			}
			w := csv.NewWriter(os.Stdout)
			err = w.Write([]string{"Item Link", "Request ID", "Request Type", "Request Subtype", "Matched type and subtype", "Cancelled in Alma"})
			if err != nil {
				return fmt.Errorf("error writing csv header: %w", err)
			}
			for _, request := range requests {
				line := []string{request.Member.Link, request.ID, request.Type, request.SubType}
				_, inMatching := matchingMap[request.Member.Link]
				if inMatching {
					line = append(line, "yes")
				} else {
					line = append(line, "no")
				}
				_, inCancelled := cancelledMap[request.Member.Link]
				if inCancelled {
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
				return fmt.Errorf("error after flushing csv: %w", err)
			}
			log.Printf("%v requests cancelled.\n", len(cancelled))
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when cancelling requests on members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			return nil
		},
	}
}
