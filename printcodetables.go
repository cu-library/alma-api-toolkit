// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"flag"
	"fmt"
)

func (m SubcommandMap) addPrintCodeTables() {
	fs := flag.NewFlagSet("print-code-tables", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Print the contents of the library and circdesk code tables.")
	}
	m[fs.Name()] = &Subcommand{
		ReadAccess: []string{"/almaws/v1/conf"},
		FlagSet:    fs,
		Run: func(requester Requester) []error {
			return nil
		},
	}
}
