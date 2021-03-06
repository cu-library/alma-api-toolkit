// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package dump provides output from the API about Alma configuration.
package dump

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/cu-library/almatoolkit/api"
	"github.com/cu-library/almatoolkit/subcommand"
)

// Config returns a new subcommand config.
func Config(envPrefix string) *subcommand.Config {
	fs := flag.NewFlagSet("conf-dump", flag.ExitOnError)
	fs.Usage = func() {
		description := "Print the output of the library and departments endpoints, and the known code tables.\n" +
			"The list of known code tables comes from:\n" +
			"https://developers.exlibrisgroup.com/blog/almas-code-tables-api-list-of-code-tables/\n" +
			"This command is meant to help run other subcommands which sometimes need a particular\n" +
			"code from a code table or the code for a library or department."
		subcommand.UsageNoFlags(fs, description)
	}
	return &subcommand.Config{
		ReadAccess: []string{"/almaws/v1/conf"},
		FlagSet:    fs,
		Run: func(ctx context.Context, c *api.Client) error {
			libraries, err := c.Libraries(ctx)
			if err != nil {
				return err
			}
			fmt.Println("Libraries:")
			for _, library := range libraries.Libraries {
				fmt.Printf("%v (%v)\n", library.Code, library.Name)
				fmt.Printf("Description: %v\n", library.Description)
				fmt.Printf("Resource Sharing: %v\n", library.ResourceSharing)
				fmt.Printf("Campus: %v (%v)\n", library.Campus.Text, library.Campus.Desc)
				fmt.Printf("Proxy: %v\n", library.Proxy)
				fmt.Printf("Default Location: %v (%v)\n", library.DefaultLocation.Text, library.DefaultLocation.Desc)
				fmt.Println()
			}
			departments, err := c.Departments(ctx)
			if err != nil {
				return err
			}
			fmt.Println("Departments:")
			for _, department := range departments.Departments {
				fmt.Printf("%v (%v)\n", department.Code, department.Name)
				fmt.Printf("Type: %v (%v)\n", department.Type.Text, department.Type.Desc)
				fmt.Printf("Work Days: %v\n", department.WorkDays)
				fmt.Printf("Printer: %v (%v)\n", department.Printer.Text, department.Printer.Desc)
				fmt.Printf("Owner: %v (%v)\n", department.Owner.Text, department.Owner.Desc)
				fmt.Println("Served Libraries:")
				for _, library := range department.ServedLibraries.Library {
					fmt.Printf("  %v (%v)\n", library.Text, library.Desc)
				}
				fmt.Println("Operators:")
				for _, operator := range department.Operators.Operator {
					fmt.Printf("  %v (%v)\n", operator.PrimaryID, operator.FullName)
				}
				fmt.Println()
			}
			tables, errs := c.CodeTables(ctx)
			fmt.Println("Code Tables:")
			for _, table := range tables {
				fmt.Printf("%v (%v)\n", table.Name, table.Description)
				fmt.Printf("Subsystem: %v (%v)\n", table.SubSystem.Text, table.SubSystem.Desc)
				fmt.Printf("Patron Facing: %v\n", table.PatronFacing)
				fmt.Printf("Language: %v (%v)\n", table.Language.Text, table.Language.Desc)
				fmt.Println("Scope:")
				fmt.Printf("  Institution : %v (%v)\n", table.Scope.InstitutionID.Text, table.Scope.InstitutionID.Desc)
				fmt.Printf("  Library : %v (%v)\n", table.Scope.LibraryID.Text, table.Scope.LibraryID.Desc)
				fmt.Println("Rows:")
				for _, row := range table.Rows {
					fmt.Printf("%v (%v) Default: %v Enabled: %v\n", row.Code, row.Description, row.Default, row.Enabled)
				}
				fmt.Println()
			}
			if len(errs) != 0 {
				for _, err := range errs {
					log.Println(err)
				}
				return fmt.Errorf("%v error(s) occured when dumping config", len(errs))
			}
			return nil
		},
	}
}
