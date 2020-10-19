// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

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

// Requester defines functions which send the request and returns the body bytes and error.
type Requester func(*http.Request) ([]byte, error)

// Subcommand stores information about subcommands.
type Subcommand struct {
	ReadAccess    []string                // The API endpoints which will require read-only access.
	WriteAccess   []string                // The API endpoints which will require write access.
	FlagSet       *flag.FlagSet           // The Flag set for this subcommand.
	ValidateFlags func() error            // A function which validates that the flagset is valid after it is parsed.
	Run           func(Requester) []error // Call this function for this subcommand.
}

// SubcommandMap maps the string from the command line to the properties of a subcommand.
// In practice, the key is always the same as the FlagSet's name.
type SubcommandMap map[string]*Subcommand

func main() {
	// Set the prefix of the default logger to the empty string.
	log.SetFlags(0)

	// Define the command line flags
	key := flag.String("key", "", "The Alma API key. Required.")
	server := flag.String("server", DefaultAlmaAPIURL, "The Alma API server to use.")
	printVersion := flag.Bool("version", false, "Print the version then exit.")
	printHelp := flag.Bool("help", false, "Print help for this command then exit.")

	// Subcommands this tool understands.
	subcommands := SubcommandMap{}
	subcommands.addPrintCodeTables()
	subcommands.addHoldingsCleanUpCallNumbers()
	subcommands.addItemsViewRequests()
	subcommands.addItemsCancelRequests()
	subcommands.addItemsScanIn()

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%v\n", ProjectName)
		fmt.Fprintf(flag.CommandLine.Output(), "Version %v\n", version)
		fmt.Fprintf(flag.CommandLine.Output(), "%v [FLAGS] subcommand [SUBCOMMAND FLAGS]\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "  Environment variables read when flag is unset:")
		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(flag.CommandLine.Output(), "  %v%v\n", EnvPrefix, strings.ToUpper(f.Name))
		})
		fmt.Fprintln(flag.CommandLine.Output(), "Subcommands:")
		for name, sub := range subcommands {
			fmt.Fprintf(flag.CommandLine.Output(), "%v\n", name)
			if sub.FlagSet != nil {
				sub.FlagSet.Usage()
			}
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

	// Was a subcommand provided? Was it valid?
	if len(flag.Args()) == 0 {
		log.Println("FATAL: A subcommand is required.")
		flag.Usage()
		os.Exit(1)
	}
	subName := flag.Args()[0]
	sub, valid := subcommands[subName]
	if !valid {
		log.Printf("FATAL: \"%v\" is not a valid subcommand.\n", subName)
		flag.Usage()
		os.Exit(1)
	}

	// Ignore errors; FlagSets are all set for ExitOnError.
	_ = sub.FlagSet.Parse(flag.Args()[1:])
	// If any flags have not been set, see if there are
	// environment variables that set them.
	err = overridefromenv.Override(sub.FlagSet, subcommandEnvPrefix(EnvPrefix, subName))
	if err != nil {
		log.Fatalln(err)
	}
	if sub.ValidateFlags != nil {
		err = sub.ValidateFlags()
		if err != nil {
			log.Printf("FATAL: %v.\n", err)
			flag.Usage()
			os.Exit(1)
		}
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

	// Our shared http client.
	client := &http.Client{}
	requestFunc := MakeRequestFunc(ctx, client, remAPICalls, *server, *key)

	err = CheckAPIandKey(requestFunc, sub.ReadAccess, sub.WriteAccess)
	if err != nil {
		cancel()
		wg.Wait()
		log.Printf("FATAL: API Check failed, %v.\n", err)
		os.Exit(1)
	}

	errs := sub.Run(requestFunc)
	if len(errs) != 0 {
		cancel()
		wg.Wait()
		log.Println("FATAL: Error(s) occured:")
		for _, err := range errs {
			log.Println("  ", err)
		}
		os.Exit(1)
	}

	cancel()
	wg.Wait()
	os.Exit(0)
}
