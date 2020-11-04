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
func Config() *subcommand.Config {
	fs := flag.NewFlagSet("items-requests", flag.ExitOnError)
	ID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	name := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  View requests on items in the given set.")
	}
	return &subcommand.Config{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
		FlagSet:     fs,
		ValidateFlags: func() error {
			return subcommand.ValidateSetNameAndSetIDFlags(*name, *ID)
		},
		Run: func(ctx context.Context, c *api.Client) error {
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
				return fmt.Errorf("an error occured when retrieving the members of %v (ID %v)", set.Name, set.ID)
			}
			requests, errs := c.ItemMembersUserRequests(ctx, members)
			fmt.Printf("User requests on members of set %v (%v).\n", set.Name, set.ID)
			fmt.Println()
			typeSubTypeCount := map[string]int{}
			for _, request := range requests {
				typeSubType := fmt.Sprintf("Type: %v Subtype: %v", request.Type, request.SubType)
				typeSubTypeCount[typeSubType] = typeSubTypeCount[typeSubType] + 1
			}
			for typeSubType, count := range typeSubTypeCount {
				fmt.Println(typeSubType, "Count:", count)
			}
			w := csv.NewWriter(os.Stdout)
			err = w.Write([]string{"Item Link", "Request ID", "Request Type", "Request Subtype"})
			if err != nil {
				return fmt.Errorf("error writing csv header: %w", err)
			}
			for _, request := range requests {
				err := w.Write([]string{request.Member.Link, request.ID, request.Type, request.SubType})
				if err != nil {
					return fmt.Errorf("error writing line to csv: %w", err)
				}
			}
			w.Flush()
			err = w.Error()
			if err != nil {
				return fmt.Errorf("error after flushing csv: %w", err)
			}
			fmt.Println()
			fmt.Printf("%v requests found.\n", len(requests))
			if len(errs) != 0 {
				fmt.Printf("\n %v Errors:\n", len(errs))
				for _, err := range errs {
					fmt.Println(err)
				}
				return fmt.Errorf("at least one error occured when retrieving requests on members of %v (ID %v)", set.Name, set.ID)
			}
			return nil
		},
	}
}
