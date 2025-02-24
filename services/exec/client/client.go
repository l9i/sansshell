/* Copyright (c) 2019 Snowflake Inc. All rights reserved.

   Licensed under the Apache License, Version 2.0 (the
   "License"); you may not use this file except in compliance
   with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing,
   software distributed under the License is distributed on an
   "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
   KIND, either express or implied.  See the License for the
   specific language governing permissions and limitations
   under the License.
*/

// Package client provides the client interface for 'exec'
package client

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Snowflake-Labs/sansshell/client"
	pb "github.com/Snowflake-Labs/sansshell/services/exec"
	"github.com/Snowflake-Labs/sansshell/services/util"
	"github.com/google/subcommands"
)

const subPackage = "exec"

func init() {
	subcommands.Register(&execCmd{}, subPackage)
}

func setup(f *flag.FlagSet) *subcommands.Commander {
	c := client.SetupSubpackage(subPackage, f)
	c.Register(&runCmd{}, "")
	return c
}

type execCmd struct{}

func (*execCmd) Name() string { return subPackage }
func (p *execCmd) Synopsis() string {
	return client.GenerateSynopsis(setup(flag.NewFlagSet("", flag.ContinueOnError)))
}
func (p *execCmd) Usage() string {
	return client.GenerateUsage(subPackage, p.Synopsis())
}
func (*execCmd) SetFlags(f *flag.FlagSet) {}

func (p *execCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	c := setup(f)
	return c.Execute(ctx, args...)
}

type runCmd struct{}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "Run provided command and return a response." }
func (*runCmd) Usage() string {
	return `run <command> [<args>...]:
  Run a command remotely and return the response

	Note: This is not optimized for large output or long running commands.  If
	the output doesn't fit in memory in a single proto message or if it doesnt
	complete within the timeout, you'll have a bad time.
`
}

func (p *runCmd) SetFlags(f *flag.FlagSet) {}

func (p *runCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	state := args[0].(*util.ExecuteState)
	if f.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Please specify a command to execute.\n")
		return subcommands.ExitUsageError
	}

	c := pb.NewExecClientProxy(state.Conn)

	resp, err := c.RunOneMany(ctx, &pb.ExecRequest{Command: f.Args()[0], Args: f.Args()[1:]})
	if err != nil {
		// Emit this to every error file as it's not specific to a given target.
		for _, e := range state.Err {
			fmt.Fprintf(e, "Could not execute due to likely program failure: %v\n", err)
		}
		return subcommands.ExitFailure
	}

	returnCode := subcommands.ExitSuccess
	for r := range resp {
		if len(r.Resp.Stderr) > 0 {
			fmt.Fprintf(state.Err[r.Index], "%s", r.Resp.Stderr)
		}
		if r.Error != nil {
			fmt.Fprintf(state.Err[r.Index], "Command execution failure for target %s (%d) - error - %v\n", r.Target, r.Index, r.Error)
			returnCode = subcommands.ExitFailure
			continue
		}
		fmt.Fprintf(state.Out[r.Index], "%s", r.Resp.Stdout)
	}
	return returnCode
}
