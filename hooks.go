package main

import "os/exec"

// Output
type HookOutput struct {
	output []byte // Output of the commands
	err    error  // Error information as returned by the hook
}

// This implements an interface to external hooks
type HookRunner interface {
	Run(...string) HookOutput
}

// Hooks to external scripts
type hookRunnerCmdExec struct {
	cmd *string
}

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

func NewScriptHookr(hook hookFunc) HookRunner {
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
