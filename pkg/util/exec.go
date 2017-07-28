/*
Copyright (c) 2017 OpenStack Foundation.

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

package util

import (
	"os/exec"
	"strings"
)

func absPath(cmd string) (string, error) {
	cmdAbsPath, err := exec.LookPath(cmd)
	if err != nil {
		return "", err
	}

	return cmdAbsPath, nil
}

func buildCommand(cmd string, args ...string) (*exec.Cmd, error) {
	cmdAbsPath, err := absPath(cmd)
	if err != nil {
		return nil, err
	}

	command := exec.Command(cmdAbsPath)
	command.Args = append(command.Args, args...)
	return command, nil
}

func RunCommand(cmd string, args ...string) ([]string, error) {
	command, err := buildCommand(cmd, args...)
	if err != nil {
		return nil, err
	}

	output, err := command.CombinedOutput()
	if err != nil {
		return []string{string(output)}, err
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}
