// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package requests provides a subcommand which outputs the requests on items in a set.
package requests

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
func Config(envPrefix string) *subcommand.Config {
	fs := flag.NewFlagSet("items-requests", flag.ExitOnError)
	ID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	name := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	fs.Usage = func() {
		description := "View requests on items in the given set."
		subcommand.Usage(fs, envPrefix, description)
	}
	return &subcommand.Config{
		ReadAccess: []string{"/almaws/v1/conf", "/almaws/v1/bibs"},
		FlagSet:    fs,
		ValidateFlags: func() error {
			return subcommand.ValidateSetNameAndSetIDFlags(*name, *ID)
		},
		Run: func(ctx context.Context, c *api.Client) error {
			set, err := c.SetFromNameOrID(ctx, *name, *ID)
			if err != nil {
				return err
			}
			if set.Type != "ITEMIZED" || set.Content != "ITEM" {
				return fmt.Errorf("the set must be an itemized set of items")
			}
			members, errs := c.SetMembers(ctx, set)
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving the members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			requests, errs := c.ItemMembersUserRequests(ctx, members)
			typeSubTypeCount := map[string]int{}
			for _, request := range requests {
				typeSubType := fmt.Sprintf("Type: %v Subtype: %v", request.Type, request.SubType)
				typeSubTypeCount[typeSubType] = typeSubTypeCount[typeSubType] + 1
			}
			for typeSubType, count := range typeSubTypeCount {
				log.Println(typeSubType, "Count:", count)
			}
			w := csv.NewWriter(os.Stdout)
			err = w.Write([]string{"Request Link", "Request Type", "Request Subtype"})
			if err != nil {
				return fmt.Errorf("error writing csv header: %w", err)
			}
			for _, request := range requests {
				err := w.Write([]string{request.Link, request.Type, request.SubType})
				if err != nil {
					return fmt.Errorf("error writing line to csv: %w", err)
				}
			}
			w.Flush()
			err = w.Error()
			if err != nil {
				return fmt.Errorf("error after flushing csv: %w", err)
			}
			log.Printf("%v requests found.\n", len(requests))
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when retrieving requests on members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			return nil
		},
	}
}
