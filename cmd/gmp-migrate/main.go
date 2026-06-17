package main

import (
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/migrate"
	"github.com/alecthomas/kingpin/v2"
)

var (
	// Flags.
	inputFile = kingpin.Flag("file", "Input source (YAML file, directory, or '-' for stdin)").Short('f').Required().String()
)

func main() {
	kingpin.CommandLine.Help = "Migrate Prometheus Operator configurations to Google Managed Prometheus (GMP)."
	kingpin.Parse()

	migrator := migrate.NewMigrator()
	_, err := migrator.Run(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Migration failed: %v\n", err)
		os.Exit(1)
	}
}
