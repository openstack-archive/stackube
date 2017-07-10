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
