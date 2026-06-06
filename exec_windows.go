//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func startProcess(proj string, name string, svc Service, replicaI int, extraEnv []string) (*ProcessHandle, error) {
	logPath := logPath(proj, name)
	logFile, err := os.OpenFile(logPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	ts := fmt.Sprintf("\n=== %s[%d] starting at %s ===\n",
		name, replicaI, time.Now().Format(time.RFC3339))
	logFile.WriteString(ts)
	logFile.Close()

	workdir := proj
	if svc.Workdir != "" {
		if filepath.IsAbs(svc.Workdir) {
			workdir = svc.Workdir
		} else {
			workdir = filepath.Join(proj, svc.Workdir)
		}
	}

	parts := splitCommand(svc.Command)
	cmdName := parts[0]
	cmdArgs := parts[1:]
	cmdArgs = append(cmdArgs, svc.Args...)

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Dir = workdir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	logWriter, err := os.OpenFile(logPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log for writing: %w", err)
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	port := svc.Port
	if port > 0 {
		port = port + replicaI
	}

	cmd.Env = os.Environ()
	for k, v := range svc.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("FLECT_PROJECT=%s", proj),
		fmt.Sprintf("FLECT_SERVICE=%s", name),
		fmt.Sprintf("FLECT_REPLICA=%d", replicaI),
	)
	cmd.Env = append(cmd.Env, extraEnv...)

	if port > 0 {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("PORT=%d", port),
			fmt.Sprintf("FLECT_%s_PORT=%d",
				strings.ToUpper(strings.ReplaceAll(name, "-", "_")), port),
		)
	}

	if err := cmd.Start(); err != nil {
		logWriter.Close()
		return nil, fmt.Errorf("starting %s[%d]: %w", name, replicaI, err)
	}

	return &ProcessHandle{
		Cmd:      cmd,
		Service:  name,
		ReplicaI: replicaI,
		Port:     port,
		Started:  time.Now(),
	}, nil
}

func stopProcess(handle *ProcessHandle, signal string) error {
	if handle == nil || handle.Cmd == nil || handle.Cmd.Process == nil {
		return nil
	}
	return handle.Cmd.Process.Signal(parseSignal(signal))
}

func waitProcess(handle *ProcessHandle, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- handle.Cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		handle.Cmd.Process.Kill()
		return <-done
	}
}

func processExited(handle *ProcessHandle) bool {
	if handle == nil || handle.Cmd == nil || handle.Cmd.ProcessState != nil {
		return handle == nil || handle.Cmd == nil || handle.Cmd.ProcessState.Exited()
	}
	return false
}

func init() {
	if os.Getenv("COMSPEC") == "" {
		os.Setenv("COMSPEC", "cmd.exe")
	}
}
