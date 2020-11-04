// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package subcommand defines commands in the Alma toolkit.
package subcommand

import (
	"context"
	"flag"
	"fmt"

	"github.com/cu-library/almatoolkit/api"
)

// Config stores information about subcommands.
type Config struct {
	ReadAccess    []string                                 // The API endpoints which will require read-only access.
	WriteAccess   []string                                 // The API endpoints which will require write access.
	FlagSet       *flag.FlagSet                            // The Flag set for this subcommand.
	ValidateFlags func() error                             // A function which validates that the flagset is valid after it is parsed.
	Run           func(context.Context, *api.Client) error // Call this function for this subcommand.
}

// Registry maps the string from the command line to the properties of a subcommand.
// The key is always the same as the FlagSet's name.
type Registry map[string]*Config

// Register the config with the registry.
func (r Registry) Register(c *Config) {
	r[c.FlagSet.Name()] = c
}

// ValidateSetNameAndSetIDFlags ensures set name XOR set ID.
func ValidateSetNameAndSetIDFlags(name, ID string) error {
	if name == "" && ID == "" {
		return fmt.Errorf("a set name or a set ID are required")
	}
	if name != "" && ID != "" {
		return fmt.Errorf("a set name OR a set ID can be provided, not both")
	}
	return nil
}
