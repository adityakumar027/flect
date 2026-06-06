//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func init() {
	// Ensure COMSPEC is available for shell operations
	if os.Getenv("COMSPEC") == "" {
		os.Setenv("COMSPEC", "cmd.exe")
	}
}
