package main

import (
	"encoding/json"
	"os/exec"
)

// HookOutput wraps the exit code, and the output, of an executed
// external hoo
type HookOutput struct {
	output []byte // Output of the commands
	err    error  // Error information as returned by the hook
}

func (ho HookOutput) Error() string {
	return "hook run failed, " + ho.err.Error() + ", output was: " + string(ho.output)
}

// MarshalJSON fulfills the json.Marshaler interface so that we can serialize
// the hook output to json for returning to the user
func (ho HookOutput) MarshalJSON() (j []byte, err error) {
	if ho.err == nil {
		resp := string(ho.output)
		j, err = json.Marshal(resp)
	} else {
		resp := ho.Error()
		j, err = json.Marshal(resp)
	}
	return
}

// HookRunner is an itnerface for running external hooks
type HookRunner interface {
	Run(...string) HookOutput
}

// Hooks to external scripts
type hookRunnerCmdExec struct {
	cmd *string
}

// NewScriptHook creates a hook to run external commands and return thier
// output and exit status
func NewScriptHook(cmd *string) HookRunner {
	result := hookRunnerCmdExec{cmd: cmd}

	return result
}

func (h hookRunnerCmdExec) Run(args ...string) HookOutput {
	var result HookOutput

	if h.cmd != nil && *h.cmd != "" {
		cmd := exec.Command(*h.cmd)
		cmd.Args = args
		result.output, result.err = cmd.CombinedOutput()
	}
	return result
}

// Hooks that are internal functions
//, primarily intended for testing
type hookRunnerFuncExec struct {
	f hookFunc
}

type hookFunc func([]string) ([]byte, error)

// NewFuncHook creates  a hook that will execute the provided function
func NewFuncHook(hook hookFunc) HookRunner {
	newhook := hookRunnerFuncExec{f: hook}

	return newhook
}

func (h hookRunnerFuncExec) Run(args ...string) HookOutput {
	var result HookOutput

	if h.f != nil {
		result.output, result.err = h.f(args)
	}
	return result
}
