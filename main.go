// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/cu-library/overridefromenv"
)

const (
	// ProjectName is the name of the executable, as displayed to the user in usage and version messages.
	ProjectName string = "The Alma API Toolkit"

	// EnvPrefix is the prefix for environment variables which override unset flags.
	EnvPrefix string = "ALMAAPITOOLKIT_"

	// DefaultAlmaAPIURL is the default Alma API Server.
	DefaultAlmaAPIURL = "api-ca.hosted.exlibrisgroup.com"

	// RemainingAPICallsThreshold is the minimum number of API calls remaining before the tool automatically stops working.
	RemainingAPICallsThreshold = 50000
)

// A version flag, which should be overwritten when building using ldflags.
var version = "devel"

// SubcommandProperties stores information about subcommands.
type SubcommandProperties struct {
	ReadAccess  []string // The API endpoints which will require read-only access.
	WriteAccess []string // The API endpoints which will require write access.
}

// validSubcommands is a map of subcommands this tool understands.
var validSubcommands = map[string]SubcommandProperties{
	"holdings-clean-up-call-numbers": SubcommandProperties{
		ReadAccess:  []string{"/almaws/v1/conf"},
		WriteAccess: []string{"/almaws/v1/bibs"},
	},
}

func main() {
	// Set the prefix of the default logger to the empty string.
	log.SetFlags(0)

	// Define the command line flags
	key := flag.String("key", "", "The Alma API key. Required.")
	server := flag.String("server", DefaultAlmaAPIURL, "The Alma API server to use.")
	dryrun := flag.Bool("dryrun", false, "Do not perform any updates. Report on what changes would have been made.")
	setID := flag.String("setid", "", "The ID of the set we are processing. This flag or setname are required.")
	setName := flag.String("setname", "", "The name of the set we are processing. This flag or setid are required.")
	printVersion := flag.Bool("version", false, "Print the version then exit.")
	printHelp := flag.Bool("help", false, "Print help for this command then exit.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%v\n", ProjectName)
		fmt.Fprintf(flag.CommandLine.Output(), "Version %v\n", version)
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "  Environment variables read when flag is unset:")

		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(flag.CommandLine.Output(), "  %v%v\n", EnvPrefix, strings.ToUpper(f.Name))
		})

		fmt.Fprintln(flag.CommandLine.Output(), "Subcommands:")
		for subcommand := range validSubcommands {
			fmt.Fprintf(flag.CommandLine.Output(), "  %v\n", subcommand)
		}
	}

	// Process the flags.
	flag.Parse()

	// Quick exit for help and version flags
	if *printVersion {
		fmt.Printf("%v - Version %v.\n", ProjectName, version)
		os.Exit(0)
	}
	if *printHelp {
		flag.CommandLine.SetOutput(os.Stdout)
		flag.Usage()
		os.Exit(0)
	}

	// If any flags have not been set, see if there are
	// environment variables that set them.
	err := overridefromenv.Override(flag.CommandLine, EnvPrefix)
	if err != nil {
		log.Fatalln(err)
	}

	// Check that required flags are set.
	if *key == "" {
		log.Fatal("FATAL: An Alma API key is required.")
	}
	if *setName == "" && *setID == "" {
		log.Fatal("FATAL: A set name or a set ID are required.")
	}
	if *setName != "" && *setID != "" {
		log.Fatal("FATAL: A set name OR a set ID can be provided, not both.")
	}

	// Was a subcommand provided? Was it valid?
	if len(flag.Args()) == 0 {
		log.Println("FATAL: A subcommand is required.")
		flag.Usage()
		os.Exit(1)
	}
	subName := flag.Args()[0]
	subProperties, valid := validSubcommands[subName]
	if !valid {
		log.Printf("FATAL: \"%v\" is not a valid subcommand.\n", subName)
		flag.Usage()
		os.Exit(1)
	}

	// Keep track of child goroutines.
	var wg sync.WaitGroup

	// Our base context, used to derive all other contexts and propigrate cancel signals.
	ctx, cancel := context.WithCancel(context.Background())

	// A channel on which the number of remaining API calls is sent.
	remAPICalls := make(chan int)

	// Cancel the base context if SIGINT or SIGTERM are recieved.
	wg.Add(1)
	go func() {
		defer wg.Done()
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigs:
			log.Println("Cancelling...")
			cancel()
		case <-ctx.Done():
		}
	}()

	// Cancel the base context if the number of remaining API calls falls below the threshold.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case remAPICalls := <-remAPICalls:
				if remAPICalls <= RemainingAPICallsThreshold {
					log.Printf("FATAL: API call threshold reached, only %v calls remaining.\n", remAPICalls)
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	if *dryrun {
		log.Println("Running in dryrun mode, no changes will be made.")
	}

	// Our shared http client.
	client := &http.Client{}

	err = CheckAPIandKey(ctx, client, remAPICalls, *server, *key, subProperties.ReadAccess, subProperties.WriteAccess)
	if err != nil {
		cancel()
		wg.Wait()
		log.Printf("FATAL: API Check failed, %v.\n", err)
		os.Exit(1)
	}

	if *setID == "" && *setName != "" {
		*setID, err = GetSetIDFromName(ctx, client, remAPICalls, *server, *key, *setName)
		if err != nil {
			cancel()
			wg.Wait()
			log.Printf("FATAL: Getting set ID from set name failed, %v.\n", err)
			os.Exit(1)
		}
		log.Printf("ID %v found for set name %v.", *setID, *setName)
	}

	numMembers, err := GetNumberOfMembers(ctx, client, remAPICalls, *server, *key, *setID)
	if err != nil {
		cancel()
		wg.Wait()
		log.Printf("FATAL: Getting number of set members from set ID failed, %v.\n", err)
		os.Exit(1)
	}
	log.Println(numMembers)

	memberIDs, err := GetMemberIDs(ctx, client, remAPICalls, *server, *key, *setID, numMembers)
	if err != nil {
		cancel()
		wg.Wait()
		log.Printf("FATAL: Getting member IDs from set failed, %v.\n", err)
		os.Exit(1)
	}
	log.Println(memberIDs)

	cancel()
	wg.Wait()
	os.Exit(0)
}
