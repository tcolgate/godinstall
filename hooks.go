package main

import "os/exec"

// This implements an interface to external hooks
type HookRunner interface {
	Run(...string) error
}

// Hooks to external scripts
type hookRunnerCmdExec struct {
	cmd *string
}

func NewScriptHook(cmd *string) HookRunner {
	newhook := hookRunnerCmdExec{cmd: cmd}

	return newhook
}

func (h hookRunnerCmdExec) Run(args ...string) (err error) {
	if h.cmd != nil && *h.cmd != "" {
		cmd := exec.Command(*h.cmd)
		cmd.Args = args
		err = cmd.Run()
	}
	return
}

// Hooks that are internal functions
//, primarily intended for testing
type hookRunnerFuncExec struct {
	f hookFunc
}

type hookFunc func([]string) error

func NewScriptHookr(hook hookFunc) HookRunner {
	newhook := hookRunnerFuncExec{f: hook}

	return newhook
}

func (h hookRunnerFuncExec) Run(args ...string) (err error) {
	if h.f != nil {
		err = h.f(args)
	}
	return
}
