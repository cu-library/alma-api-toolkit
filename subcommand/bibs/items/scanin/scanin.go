// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package scanin provides a subcommand which scans in items in a set.
package scanin

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
	fs := flag.NewFlagSet("items-scan-in", flag.ExitOnError)
	ID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	name := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	circdesk := fs.String("circdesk", api.DefaultCircDesk, "The circ desk code. The possible values are not available through the API, "+
		"see https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_item_loan.xsd/?tags=GET.")
	library := fs.String("library", "", "The library code. Use the conf-libaries-departments-code-tables subcommand to see the possible values.")
	dryrun := fs.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	fs.Usage = func() {
		description := "Scan the members of a set of items in."
		subcommand.Usage(fs, envPrefix, description)
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
			if *circdesk == "" {
				return fmt.Errorf("a circ desk code is required")
			}
			if *library == "" {
				return fmt.Errorf("a library code is required")
			}
			return nil
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
			items := []api.Item{}
			errs = []error{}
			if !*dryrun {
				items, errs = c.ItemMembersScanIn(ctx, members, *circdesk, *library)
			}
			scannedInMap := map[string]bool{}
			for _, item := range items {
				scannedInMap[item.Barcode] = true
			}
			w := csv.NewWriter(os.Stdout)
			err = w.Write([]string{"MMS ID", "Title", "Author", "Call Number", "Barcode", "Scanned in in Alma"})
			if err != nil {
				return fmt.Errorf("error writing csv header: %w", err)
			}
			for _, item := range items {
				line := []string{item.MMSID, item.Title, item.Author, item.CallNumber, item.Barcode}
				_, inScannedIn := scannedInMap[item.Barcode]
				if inScannedIn {
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
			log.Printf("%v successful scan in operations.\n", len(items))
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when scanning in members of '%v' (ID %v)", len(errs), set.Name, set.ID)
			}
			return nil
		},
	}
}
