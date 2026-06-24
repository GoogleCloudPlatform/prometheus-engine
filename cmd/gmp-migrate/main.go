// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/migrate"
)

// commaStringSlice implements the flag.Value interface to support
// repeated and/or comma-separated string flags.
type commaStringSlice []string

func (s *commaStringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *commaStringSlice) Set(value string) error {
	for p := range strings.SplitSeq(value, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(migrate.NewConsoleHandler(os.Stderr)))

	var inputFiles commaStringSlice
	flag.Var(&inputFiles, "file", "Input source (YAML file, directory, or '-' for stdin) (Required)")
	flag.Var(&inputFiles, "f", "Input source (YAML file, directory, or '-' for stdin) (Required)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprint(os.Stderr, "Migrate Prometheus Operator configurations to Google Managed Prometheus (GMP).\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Reject unexpected positional arguments to prevent silent typos (like forgetting -f)
	if flag.NArg() > 0 {
		slog.Error("Unexpected positional arguments.",
			slog.Any("arguments", flag.Args()),
		)
		fmt.Fprintln(os.Stderr, "\nAll input files and directories must be explicitly passed using the -f or --file flags. For example:")
		var allInputs []string
		allInputs = append(allInputs, inputFiles...)
		allInputs = append(allInputs, flag.Args()...)
		fmt.Fprintf(os.Stderr, "  %s -f %s\n\n", os.Args[0], strings.Join(allInputs, " -f "))
		flag.Usage()
		os.Exit(1)
	}

	if len(inputFiles) == 0 {
		slog.Error("Flag -f / --file is required.")
		flag.Usage()
		os.Exit(1)
	}

	migrator := migrate.NewMigrator()
	report, err := migrator.Run(inputFiles...)
	if err != nil {
		slog.Error("Migration failed", slog.Any("error", err))
		os.Exit(1)
	}

	// If any resource failed to migrate, exit with a non-zero code.
	if report.FailedCount > 0 {
		slog.Error("Migration completed with failures", slog.Int("failures", report.FailedCount))
		os.Exit(1)
	}
}
