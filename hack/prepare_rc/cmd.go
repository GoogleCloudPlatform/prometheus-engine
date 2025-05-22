// Copyright 2025 Google LLC
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
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Command struct {
	command string
	args    []string
}

func Cmd(command string, args ...string) *Command {
	return &Command{
		command: command,
		args:    args,
	}
}

func (c *Command) String() string {
	if c == nil {
		return "<nil Command>"
	}
	return fmt.Sprintf("%s %s", c.command, strings.Join(c.args, " "))
}

func (c *Command) run() (string, error) {
	cmd := exec.Command(c.command, c.args...)

	var stdoutBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &stdoutBuf)
	cmd.Stderr = os.Stdout

	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to start command '%s': %w",
			c, err)
	}
	waitErr := cmd.Wait()
	result := strings.TrimSpace(stdoutBuf.String())

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return result, fmt.Errorf("command '%s' exited with code %d: %w",
				c, exitErr.ExitCode(), exitErr)
		}
		return result, fmt.Errorf("command '%s' failed after starting: %w",
			c, waitErr)
	}

	return result, nil
}

func (c *Command) Run() (string, error) {
	fmt.Printf("::group::Running: %s\n", c)
	res, err := c.run()
	fmt.Println("::endgroup::")
	if err != nil {
		fmt.Printf("::info::Command '%s' failed: %v\n", c, err)
	}
	return res, err
}
