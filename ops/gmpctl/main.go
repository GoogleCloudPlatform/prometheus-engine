// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	cfgPath        = flag.String("c", ".gmpctl.default.yaml", "Path to the configuration file. See config.go#Config for the structure.")
	verbose        = flag.Bool("v", false, "Enabled verbose, debug output (e.g. logging os.Exec commands)")
	gitPreferHTTPS = flag.Bool("git.prefer-https", false, "If true, uses HTTPS protocol instead of git for remote URLs. ")
)

type Config struct {
	// Directory for the gmpctl work, notably for project clones and git worktrees.
	Directory string `yaml:"dir"`
}

func loadConfig() (ret *Config, _ error) {
	b, err := os.ReadFile(*cfgPath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &ret); err != nil {
		return nil, err
	}
	ret.Directory, err = filepath.Abs(ret.Directory)
	return ret, err
}

func main() {
	flag.Usage = func() {
		fmt.Println("Usage: gmpctl [COMMAND] [FLAGS]")
		flag.PrintDefaults()
		fmt.Print("\n--- Commands ---\n")
		fmt.Print("[release] ")
		releaseFlags.Usage()
		fmt.Println()
		fmt.Print("[vulnfix] ")
		vulnfixFlags.Usage()
	}

	// Help.
	for _, cmd := range os.Args[1:] {
		if cmd == "-h" || cmd == "--help" {
			flag.Usage()
			os.Exit(0)
		}
	}

	flag.Parse()
	if flag.NArg() == 0 {
		errf("expected at least one argument, got none")
		flag.Usage()
		os.Exit(1)
	}

	var (
		err error
		cmd = flag.Arg(0)
	)
	switch cmd {

	case "release":
		err = release()
	case "vulnfix":
		err = vulnfix()
	default:
		errf("Unknown command: %q", cmd)
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		errf("Command %q failed: %v", cmd, err)
		os.Exit(1)
	}
	successf("Command %q succeded", cmd)
	os.Exit(0)
}

type cmdOpts struct {
	// Dir configures the directory that command will be run from.
	Dir string

	// Envs configures additional OS environments.
	Envs []string
	// HideOutputs disables stdout and stderr streaming.
	// This is useful for ~porcelain like commands that are meant to pass state via
	// stdout.
	HideOutputs bool
}

// libScriptFile is contains useful shell functions.
// Sometimes it's just easier to hack something in bash before porting to Go.
// TODO(bwplotka): go embed this for portability?
const libScriptFile = "lib.sh"

// getFromLibFunction runs certain function from libScript that's expected to pass a return
// parameter via stdout.
func getFromLibFunction(dir string, envs []string, function string, args ...string) (string, error) {
	curr, err := filepath.Abs("") // Hacky. TODO(bwplotka): Improve dir management.
	if err != nil {
		return "", err
	}
	libScript := filepath.Join(curr, libScriptFile)

	envs = append(envs,
		fmt.Sprintf("SCRIPT_DIR=%v", curr),
	)
	return runCommand(
		&cmdOpts{Dir: dir, Envs: envs, HideOutputs: true},
		"bash", "-c", fmt.Sprintf(". %v && %v %v", libScript, function, strings.Join(args, " ")),
	)
}

// runLibFunction runs certain function from libScript that is not expected
// to pass any return parameters.
func runLibFunction(dir string, envs []string, function string, args ...string) error {
	curr, err := filepath.Abs("") // Hacky. TODO(bwplotka): Improve dir management.
	if err != nil {
		return err
	}
	libScript := filepath.Join(curr, libScriptFile)

	envs = append(envs, fmt.Sprintf("SCRIPT_DIR=%v", curr))
	_, err = runCommand(
		&cmdOpts{Dir: dir, Envs: envs, HideOutputs: false},
		"bash", "-c", fmt.Sprintf(". %v && %v %v", libScript, function, strings.Join(args, " ")),
	)
	return err
}

func runLocalBash(dir string, envs []string, file string, args ...string) error {
	curr, err := filepath.Abs("") // Hacky. TODO(bwplotka): Improve dir management.
	if err != nil {
		return err
	}

	cmdArgs := []string{"bash", filepath.Join(curr, file)}
	cmdArgs = append(cmdArgs, args...)
	envs = append(envs, fmt.Sprintf("SCRIPT_DIR=%v", curr))
	_, err = runCommand(&cmdOpts{Dir: dir, Envs: envs, HideOutputs: false}, cmdArgs...)
	return err
}

// runCommand executes a command in a specific directory
func runCommand(opts *cmdOpts, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command to execute")
	}

	if opts == nil {
		opts = &cmdOpts{}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = opts.Dir
	if cmd.Dir == "" {
		cmd.Dir, _ = filepath.Abs("")
	}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, opts.Envs...)

	if *verbose {
		cmd.Env = append(cmd.Env, "DEBUG_MODE=yes")
		logf("DEBUG: Executing %q from %q", cmd.String(), cmd.Dir)
	}

	var (
		out    bytes.Buffer
		stderr bytes.Buffer
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if !opts.HideOutputs {
		cmd.Stdout = io.MultiWriter(os.Stdout, &out)
		cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	}
	if err := cmd.Run(); err != nil {
		if opts.HideOutputs {
			return "", fmt.Errorf("%v failed: %s; %s", args, err, stderr.String())
		}
		return "", fmt.Errorf("%v failed: %s", args, err)
	}
	return strings.TrimSpace(out.String()), nil
}

func logf(msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "🔄  "+msg, args...)
	_, _ = fmt.Fprintln(os.Stderr)
}

func errf(msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, " ❌  "+msg, args...)
	_, _ = fmt.Fprintln(os.Stderr)
}

func panicf(msg string, args ...any) {
	// TODO(bwplotka): Panics are much better for scripting. The alternative
	// is a strict wrapping (with extra lib). I'd suggest we panic on things
	// we know we never handle errors on.
	panic(fmt.Sprintf(" ❌  "+msg, args...))
}

func successf(msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, " ✅  "+msg+"!", args...)
	_, _ = fmt.Fprintln(os.Stderr)
}
