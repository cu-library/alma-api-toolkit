// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"flag"
	"fmt"
)

func (m SubcommandMap) addHoldingsCleanUpCallNumbers() {
	fs := flag.NewFlagSet("holdings-clean-up-call-numbers", flag.ExitOnError)
	setID := fs.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := fs.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
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
			fmt.Println("members:")
			fmt.Println(members)
			return errs
		},
	}
}
