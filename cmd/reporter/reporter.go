// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Reporter is a CLI tool to process file system report files generated by Walker.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/google/fswalker"
)

var (
	configFile = flag.String("configFile", "", "required report config file to use")
	walkPath   = flag.String("walkPath", "", "path to search for Walks")
	reviewFile = flag.String("reviewFile", "", "path to the file containing a list of last-known-good states - this needs to be writeable")
	hostname   = flag.String("hostname", "", "host to review the differences for")
	beforeFile = flag.String("beforeFile", "", "path to the file to compare against (last known good typically)")
	afterFile  = flag.String("afterFile", "", "path to the file to compare with the before state")
	paginate   = flag.Bool("paginate", false, "pipe output into $PAGER in order to paginate and make reviews easier")
	verbose    = flag.Bool("verbose", false, "print additional output for each file which changed")
)

const (
	lessCmd = "/usr/bin/less"
)

func updateReviews() bool {
	fmt.Print("Do you want to update the \"last known good\" to this [y/N]: ")
	var input string
	fmt.Scanln(&input)
	if strings.ToLower(strings.TrimSpace(input)) == "y" {
		return true
	}
	return false
}

func main() {
	ctx := context.Background()

	// Loading configs and walks.
	if *configFile == "" {
		log.Fatal("configFile needs to be specified")
	}
	rptr, err := fswalker.ReporterFromConfigFile(ctx, *configFile, *verbose)
	if err != nil {
		log.Fatal(err)
	}
	if err := rptr.LoadWalks(ctx, *hostname, *reviewFile, *walkPath, *afterFile, *beforeFile); err != nil {
		log.Fatal(err)
	}

	// Processing and output.
	// Note that we do some trickery here to allow pagination via $PAGER if requested.
	out := io.WriteCloser(os.Stdout)
	var cmd *exec.Cmd
	if *paginate {
		// Use $PAGER if it is set - if not, revert to using less.
		cmdpath := os.Getenv("PAGER")
		if cmdpath == "" {
			cmdpath = lessCmd
		}

		var err error
		cmd = exec.Command(lessCmd)
		cmd.Stdout = os.Stdout     // so less writes into stdout
		out, err = cmd.StdinPipe() // so we write into less' input
		if err != nil {
			log.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			log.Fatal(fmt.Errorf("unable to start %q: %v", lessCmd, err))
		}
	}
	rptr.PrintReportSummary(out)
	rptr.PrintRuleSummary(out)
	rptr.Compare(out)

	if *paginate {
		out.Close()
		cmd.Wait()
	}

	// Update reviews file if desired.
	if updateReviews() {
		if err := rptr.UpdateReviewProto(ctx); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("not updating reviews file")
	}

	fmt.Println()
	fmt.Println("Metrics:")
	for _, k := range rptr.Counter.Metrics() {
		v, _ := rptr.Counter.Get(k)
		fmt.Printf("[%-30s] = %6d\n", k, v)
	}
}
