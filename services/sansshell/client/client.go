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

// Package client provides the client interface for 'Logging'
package client

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/subcommands"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/Snowflake-Labs/sansshell/client"
	pb "github.com/Snowflake-Labs/sansshell/services/sansshell"
	"github.com/Snowflake-Labs/sansshell/services/util"
)

const subPackage = "sansshell"

func init() {
	subcommands.Register(&sansshellCmd{}, subPackage)
}

func setup(f *flag.FlagSet) *subcommands.Commander {
	c := client.SetupSubpackage(subPackage, f)
	c.Register(&setVerbosityCmd{}, "")
	c.Register(&getVerbosityCmd{}, "")
	c.Register(&setProxyVerbosityCmd{}, "")
	c.Register(&getProxyVerbosityCmd{}, "")
	return c
}

type sansshellCmd struct{}

func (*sansshellCmd) Name() string { return subPackage }
func (p *sansshellCmd) Synopsis() string {
	return client.GenerateSynopsis(setup(flag.NewFlagSet("", flag.ContinueOnError)))
}
func (p *sansshellCmd) Usage() string {
	return client.GenerateUsage(subPackage, p.Synopsis())
}
func (*sansshellCmd) SetFlags(f *flag.FlagSet) {}

func (p *sansshellCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	c := setup(f)
	return c.Execute(ctx, args...)
}

type setVerbosityCmd struct {
	level int
}

func (*setVerbosityCmd) Name() string     { return "set-verbosity" }
func (*setVerbosityCmd) Synopsis() string { return "Set the logging verbosity level." }
func (*setVerbosityCmd) Usage() string {
	return `set-verbosity:
  Sends an integer logging level and returns the previous integer logging level.
`
}

func (s *setVerbosityCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&s.level, "verbosity", 0, "The logging verbosity level to set")
}

func (s *setVerbosityCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	state := args[0].(*util.ExecuteState)
	c := pb.NewLoggingClientProxy(state.Conn)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.SetVerbosityOneMany(ctx, &pb.SetVerbosityRequest{Level: int32(s.level)})
	if err != nil {
		// Emit this to every error file as it's not specific to a given target.
		for _, e := range state.Err {
			fmt.Fprintf(e, "Could not set logging: %v\n", err)
		}
		return subcommands.ExitFailure
	}

	retCode := subcommands.ExitSuccess
	for r := range resp {
		if r.Error != nil {
			fmt.Fprintf(state.Err[r.Index], "Setting logging verbosity for target %s (%d) returned error: %v\n", r.Target, r.Index, r.Error)
			retCode = subcommands.ExitFailure
			continue
		}
		fmt.Fprintf(state.Out[r.Index], "Target %s (%d) previous logging level %d\n", r.Target, r.Index, r.Resp.Level)
	}
	return retCode
}

type getVerbosityCmd struct {
}

func (*getVerbosityCmd) Name() string     { return "get-verbosity" }
func (*getVerbosityCmd) Synopsis() string { return "Get logging level verbosity" }
func (*getVerbosityCmd) Usage() string {
	return `get-verbosity:
  Sends an empty request and expects to get back an integer level for the current logging verbosity.
`
}

func (*getVerbosityCmd) SetFlags(f *flag.FlagSet) {}

func (g *getVerbosityCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	state := args[0].(*util.ExecuteState)
	c := pb.NewLoggingClientProxy(state.Conn)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.GetVerbosityOneMany(ctx, &emptypb.Empty{})
	if err != nil {
		// Emit this to every error file as it's not specific to a given target.
		for _, e := range state.Err {
			fmt.Fprintf(e, "Could not set logging: %v\n", err)
		}
		return subcommands.ExitFailure
	}

	retCode := subcommands.ExitSuccess
	for r := range resp {
		if r.Error != nil {
			fmt.Fprintf(state.Err[r.Index], "Getting logging verbosity for target %s (%d) returned error: %v\n", r.Target, r.Index, r.Error)
			retCode = subcommands.ExitFailure
			continue
		}
		fmt.Fprintf(state.Out[r.Index], "Target %s (%d) current logging level %d\n", r.Target, r.Index, r.Resp.Level)
	}
	return retCode
}

type setProxyVerbosityCmd struct {
	level int
}

func (*setProxyVerbosityCmd) Name() string     { return "set-proxy-verbosity" }
func (*setProxyVerbosityCmd) Synopsis() string { return "Set the proxy logging verbosity level." }
func (*setProxyVerbosityCmd) Usage() string {
	return `set-proxy-verbosity:
  Sends an integer logging level for the proxy server and returns the previous integer logging level.
`
}

func (s *setProxyVerbosityCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&s.level, "verbosity", 0, "The logging verbosity level to set")
}

func (s *setProxyVerbosityCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	state := args[0].(*util.ExecuteState)
	if len(state.Out) > 1 {
		fmt.Fprintf(os.Stderr, "can't call proxy logging with multiple targets")
	}
	// Get a real connection to the proxy
	c := pb.NewLoggingClient(state.Conn.Proxy())

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.SetVerbosity(ctx, &pb.SetVerbosityRequest{Level: int32(s.level)})
	if err != nil {
		fmt.Fprintf(state.Err[0], "Could not set proxy logging: %v\n", err)
		return subcommands.ExitFailure
	}
	fmt.Fprintf(state.Out[0], "Proxy previous logging level %d\n", resp.Level)
	return subcommands.ExitSuccess
}

type getProxyVerbosityCmd struct {
}

func (*getProxyVerbosityCmd) Name() string     { return "get-proxy-verbosity" }
func (*getProxyVerbosityCmd) Synopsis() string { return "Get the proxy logging level verbosity" }
func (*getProxyVerbosityCmd) Usage() string {
	return `get-proxy-verbosity:
  Sends an empty request and expects to get back an integer level for the current proxy logging verbosity.
`
}

func (*getProxyVerbosityCmd) SetFlags(f *flag.FlagSet) {}

func (g *getProxyVerbosityCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	state := args[0].(*util.ExecuteState)
	if len(state.Out) > 1 {
		fmt.Fprintf(os.Stderr, "can't call proxy logging with multiple targets")
	}
	// Get a real connection to the proxy
	c := pb.NewLoggingClient(state.Conn.Proxy())

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.GetVerbosity(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Fprintf(state.Err[0], "Could not get proxy logging: %v\n", err)
		return subcommands.ExitFailure
	}
	fmt.Fprintf(state.Out[0], "Proxy current logging level %d\n", resp.Level)
	return subcommands.ExitSuccess
}
