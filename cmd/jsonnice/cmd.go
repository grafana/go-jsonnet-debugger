/*
Copyright 2019 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/cmd/internal/cmd"
	"github.com/lmittmann/tint"
)

func version(o io.Writer) {
	fmt.Fprintf(o, "Jsonnet debugger %s\n", jsonnet.Version())
}

func usage(o io.Writer) {
	version(o)
	fmt.Fprintln(o)
	fmt.Fprintln(o, "jsonnice {<option>} { <filename> }")
	fmt.Fprintln(o)
	fmt.Fprintln(o, "Available options:")
	fmt.Fprintln(o, "  -h / --help                This message")
	fmt.Fprintln(o, "  -e / --exec                Treat filename as code")
	fmt.Fprintln(o, "  -J / --jpath <dir>         Specify an additional library search dir")
	fmt.Fprintln(o, "  -d / --dap                 Start a debug-adapter-protocol server")
	fmt.Fprintln(o, "  -l / --log-level           Set the log level. Allowed values: debug,info,warn,error")
	fmt.Fprintln(o, "  --version                  Print version")
	fmt.Fprintln(o)
	fmt.Fprintln(o, "In all cases:")
	fmt.Fprintln(o, "  <filename> can be - (stdin)")
	fmt.Fprintln(o, "  Multichar options are expanded e.g. -abc becomes -a -b -c.")
	fmt.Fprintln(o, "  The -- option suppresses option processing for subsequent arguments.")
	fmt.Fprintln(o, "  Note that since filenames and jsonnet programs can begin with -, it is")
	fmt.Fprintln(o, "  advised to use -- if the argument is unknown, e.g. jsonnice -- \"$FILENAME\".")
}

type config struct {
	inputFile      string
	filenameIsCode bool
	dap            bool
	jpath          []string
	logLevel       slog.Level
}

type processArgsStatus int

const (
	processArgsStatusContinue     = iota
	processArgsStatusSuccessUsage = iota
	processArgsStatusFailureUsage = iota
	processArgsStatusSuccess      = iota
	processArgsStatusFailure      = iota
)

func processArgs(givenArgs []string, config *config) (processArgsStatus, error) {
	args := cmd.SimplifyArgs(givenArgs)
	remainingArgs := make([]string, 0, len(args))
	i := 0

	for ; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			return processArgsStatusSuccessUsage, nil
		} else if arg == "-v" || arg == "--version" {
			version(os.Stdout)
			return processArgsStatusSuccess, nil
		} else if arg == "-e" || arg == "--exec" {
			config.filenameIsCode = true
		} else if arg == "--" {
			// All subsequent args are not options.
			i++
			for ; i < len(args); i++ {
				remainingArgs = append(remainingArgs, args[i])
			}
			break
		} else if arg == "-J" || arg == "--jpath" {
			dir := cmd.NextArg(&i, args)
			if len(dir) == 0 {
				return processArgsStatusFailure, fmt.Errorf("-J argument was empty string")
			}
			config.jpath = append(config.jpath, dir)
		} else if arg == "-d" || arg == "--dap" {
			config.dap = true
		} else if arg == "-l" || arg == "--log-level" {
			level := cmd.NextArg(&i, args)
			if len(level) == 0 {
				return processArgsStatusFailure, fmt.Errorf("no log level specified")
			}
			slvl := slog.LevelError
			switch level {
			case "debug":
				slvl = slog.LevelDebug
			case "info":
				slvl = slog.LevelInfo
			case "warn":
				slvl = slog.LevelWarn
			case "error":
				slvl = slog.LevelError
			default:
				return processArgsStatusFailure, fmt.Errorf("invalid log level %s. Allowed: debug,info,warn,error", level)
			}
			config.logLevel = slvl
		} else if len(arg) > 1 && arg[0] == '-' {
			return processArgsStatusFailure, fmt.Errorf("unrecognized argument: %s", arg)
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if config.dap {
		return processArgsStatusContinue, nil
	}

	want := "filename"
	if config.filenameIsCode {
		want = "code"
	}
	if len(remainingArgs) == 0 {
		return processArgsStatusFailureUsage, fmt.Errorf("must give %s", want)
	}
	if len(remainingArgs) != 1 {
		// Should already have been caught by processArgs.
		panic("Internal error: expected a single input file.")
	}

	config.inputFile = remainingArgs[0]
	return processArgsStatusContinue, nil
}

func main() {
	config := config{
		jpath:    []string{},
		logLevel: slog.LevelError,
	}
	status, err := processArgs(os.Args[1:], &config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: "+err.Error())
	}
	switch status {
	case processArgsStatusContinue:
		break
	case processArgsStatusSuccessUsage:
		usage(os.Stdout)
		os.Exit(0)
	case processArgsStatusFailureUsage:
		if err != nil {
			fmt.Fprintln(os.Stderr, "")
		}
		usage(os.Stderr)
		os.Exit(1)
	case processArgsStatusSuccess:
		os.Exit(0)
	case processArgsStatusFailure:
		os.Exit(1)
	}

	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level: config.logLevel,
	})))

	if config.dap {
		err := dapServer("54321")
		if err != nil {
			slog.Error("dap server terminated", "err", err)
		}
		return
	}

	inputFile := config.inputFile
	input := cmd.SafeReadInput(config.filenameIsCode, &inputFile)
	if !config.filenameIsCode {
		config.jpath = append(config.jpath, path.Dir(inputFile))
	}
	repl := MakeReplDebugger(inputFile, input, config.jpath)
	repl.Run()
}
