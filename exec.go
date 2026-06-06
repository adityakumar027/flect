//go:build !windows

package main

import (
	"fmt"
	"net"
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

	attr := &syscall.SysProcAttr{Setpgid: true}
	if err := applyPlatformSecurity(attr, svc); err != nil {
		return nil, fmt.Errorf("security setup: %w", err)
	}
	cmd.SysProcAttr = attr

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
	sig := parseSignal(signal)
	pgid, err := syscall.Getpgid(handle.Cmd.Process.Pid)
	if err == nil && pgid > 0 {
		procGroup, _ := os.FindProcess(-pgid)
		if procGroup != nil {
			return procGroup.Signal(sig)
		}
	}
	return handle.Cmd.Process.Signal(sig)
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
	if handle == nil || handle.Cmd == nil || handle.Cmd.Process == nil {
		return true
	}
	return handle.Cmd.Process.Signal(syscall.Signal(0)) != nil
}

func portInUse(port int) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err == nil {
		conn.Close()
		return true
	}
	return false
}

func waitForPort(port int, timeout time.Duration) bool {
	if port <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if portInUse(port) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func findExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func checkRequiredExecutables(cfg *Config) []string {
	var missing []string
	seen := make(map[string]bool)
	for _, svc := range cfg.Services {
		parts := splitCommand(svc.Command)
		if len(parts) == 0 {
			continue
		}
		bin := parts[0]
		if seen[bin] {
			continue
		}
		seen[bin] = true
		if !findExecutable(bin) {
			missing = append(missing, bin)
		}
	}
	return missing
}

func splitCommand(cmd string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		} else if c == '\'' || c == '"' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func parseSignal(s string) os.Signal {
	switch s {
	case "SIGTERM", "term", "TERM":
		return syscall.SIGTERM
	case "SIGINT", "int", "INT":
		return syscall.SIGINT
	case "SIGKILL", "kill", "KILL":
		return syscall.SIGKILL
	case "SIGQUIT", "quit", "QUIT":
		return syscall.SIGQUIT
	case "SIGHUP", "hup", "HUP":
		return syscall.SIGHUP
	default:
		return syscall.SIGTERM
	}
}

func pidStr(h *ProcessHandle) string {
	if h == nil || h.Cmd == nil || h.Cmd.Process == nil {
		return "stopped"
	}
	return strconv.Itoa(h.Cmd.Process.Pid)
}

func init() {
	if _, err := exec.LookPath("sh"); err != nil {
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			os.Setenv("SHELL", comspec)
		}
	}
}
