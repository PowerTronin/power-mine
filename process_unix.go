//go:build linux || darwin || freebsd || openbsd || netbsd

package main

import (
	"os/exec"
	"syscall"
)

func detachCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
