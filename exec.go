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

type ProcessHandle struct {
	Cmd     *exec.Cmd
	Service string
	Port    int
	Started time.Time
}

func startProcess(proj string, name string, svc Service) (*ProcessHandle, error) {
	logPath := logPath(proj, name)
	logFile, err := os.OpenFile(logPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	logFile.WriteString(fmt.Sprintf("\n=== %s starting at %s ===\n",
		name, time.Now().Format(time.RFC3339)))
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
	cmd.SysProcAttr = sysProcAttr()

	logWriter, err := os.OpenFile(logPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log for writing: %w", err)
	}

	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	cmd.Env = os.Environ()

	for k, v := range svc.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if svc.Port > 0 {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("PORT=%d", svc.Port),
			fmt.Sprintf("FLECT_%s_PORT=%d", strings.ToUpper(name), svc.Port),
		)
	}

	if err := cmd.Start(); err != nil {
		logWriter.Close()
		return nil, fmt.Errorf("starting %s: %w", name, err)
	}

	return &ProcessHandle{
		Cmd:     cmd,
		Service: name,
		Port:    svc.Port,
		Started: time.Now(),
	}, nil
}

func stopProcess(handle *ProcessHandle, signal string) error {
	if handle == nil || handle.Cmd == nil || handle.Cmd.Process == nil {
		return nil
	}

	sig := parseSignal(signal)
	pgid, err := syscall.Getpgid(handle.Cmd.Process.Pid)
	if err == nil && pgid > 0 {
		// Send signal to entire process group
		procGroup, _ := os.FindProcess(-pgid)
		if procGroup != nil {
			procGroup.Signal(sig)
			return nil
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

func portInUse(port int) bool {
	if port <= 0 {
		return false
	}
	check := exec.Command("sh", "-c",
		fmt.Sprintf("fuser %d/tcp 2>/dev/null || ss -tlnp 2>/dev/null | grep -q ':%d ' || netstat -tlnp 2>/dev/null | grep -q ':%d '",
			port, port, port))
	return check.Run() == nil
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
	// Simple shell-like splitting. Handles quoted args.
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

func init() {
	// Ensure common unix defaults
	if _, err := exec.LookPath("sh"); err != nil {
		// On Windows, commands may need comspec
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			os.Setenv("SHELL", comspec)
		}
	}
}

// sysProcAttr returns platform-specific process attributes
// This is the unix implementation; Windows is in exec_windows.go
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func formatPID(pid int) string {
	if pid <= 0 {
		return "stopped"
	}
	return strconv.Itoa(pid)
}

func processExited(handle *ProcessHandle) bool {
	if handle == nil || handle.Cmd == nil || handle.Cmd.Process == nil {
		return true
	}
	err := handle.Cmd.Process.Signal(syscall.Signal(0))
	return err != nil
}
