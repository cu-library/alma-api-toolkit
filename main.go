// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cu-library/overridefromenv"
)

const (
	// ProjectName is the name of the executable, as displayed to the user in usage and version messages.
	ProjectName string = "The Alma API Toolkit"

	// EnvPrefix is the prefix for environment variables which override unset flags.
	EnvPrefix string = "ALMAAPITOOLKIT_"

	// DefaultAlmaAPIURL is the default Alma API Server.
	DefaultAlmaAPIURL = "api-ca.hosted.exlibrisgroup.com"
)

// A version flag, which should be overwritten when building using ldflags.
var version = "devel"

func main() {
	// Define the command line flags
	almaAPIKey := flag.String("almaapikey", "", "The Alma API key. Required.")
	almaAPIServer := flag.String("almaapi", DefaultAlmaAPIURL, "The Alma API server to use.")
	dryrun := flag.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	setID := flag.String("setid", "", "The ID of the set we are processing.")
	setName := flag.String("setname", "", "The name of the set we are processing.")
	printVersion := flag.Bool("version", false, "Print the version then exit.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%v\n", ProjectName)
		fmt.Fprintf(os.Stderr, "Version %v\n", version)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "  Environment variables read when flag is unset:")

		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(os.Stderr, "  %v%v\n", EnvPrefix, strings.ToUpper(f.Name))
		})

		fmt.Fprintln(os.Stderr, "Subcommands:")
		fmt.Fprintln(os.Stderr, "  holdings-clean-up-call-numbers")
	}

	// Process the flags.
	flag.Parse()

	// If any flags have not been set, see if there are
	// environment variables that set them.
	err := overridefromenv.Override(flag.CommandLine, EnvPrefix)
	if err != nil {
		log.Fatalln(err)
	}

	if *printVersion {
		fmt.Printf("%v - Version %v.\n", ProjectName, version)
		os.Exit(0)
	}

	if *almaAPIKey == "" {
		log.Fatal("FATAL: An Alma API key is required.")
	}
	if *setName == "" && *setID == "" {
		log.Fatal("FATAL: A set name or a set ID are required.")
	}
	if *setName != "" && *setID != "" {
		log.Fatal("FATAL: A set name OR a set ID can be provided, not both.")
	}

	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "holdings-clean-up-call-numbers":
		fmt.Println("Holdings clean up")
		fmt.Println(*almaAPIServer)
		fmt.Println(*dryrun)
	default:
		fmt.Printf("%v is not valid command.\n", os.Args[1])
		flag.Usage()
		os.Exit(2)
	}

}
