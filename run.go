package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runExec(proj string, args []string) {
	imageName := ""
	cmdArgs := args

	for i := 0; i < len(args); i++ {
		if args[i] == "--image" && i+1 < len(args) {
			imageName = args[i+1]
			cmdArgs = append(args[:i], args[i+2:]...)
			break
		}
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, fail("usage: flect run [--image <name>] <command> [args...]"))
		os.Exit(1)
	}

	fullCmd := strings.Join(cmdArgs, " ")
	parts := splitCommand(fullCmd)

	if imageName != "" {
		runWithImage(proj, imageName, parts)
		return
	}

	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading state:"), err)
		os.Exit(1)
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = proj
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		"FLECT_PROJECT="+proj,
		"FLECT_RUNNING=true",
	)

	for name, s := range st.Services {
		if s.Running {
			upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
			if s.Port > 0 {
				cmd.Env = append(cmd.Env,
					fmt.Sprintf("FLECT_%s_PORT=%d", upper, s.Port))
			}
		}
	}

	handles := snapshotProcesses()
	for name, instances := range handles {
		upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		var hosts, ports []string
		for _, h := range instances {
			hosts = append(hosts, "127.0.0.1")
			ports = append(ports, fmt.Sprintf("%d", h.Port))
		}
		if len(hosts) > 0 {
			cmd.Env = append(cmd.Env,
				fmt.Sprintf("FLECT_%s_HOSTS=%s", upper, strings.Join(hosts, ";")),
				fmt.Sprintf("FLECT_%s_PORTS=%s", upper, strings.Join(ports, ";")),
			)
			if len(hosts) == 1 {
				cmd.Env = append(cmd.Env,
					fmt.Sprintf("FLECT_%s_HOST=%s", upper, "127.0.0.1"),
					fmt.Sprintf("FLECT_%s_PORT=%s", upper, ports[0]),
				)
			}
		}
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("run error:"), err)
		os.Exit(1)
	}
}

func runWithImage(proj, imageName string, cmdParts []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("home directory:"), err)
		os.Exit(1)
	}

	imgDir := filepath.Join(home, ".flect", "images", imageName)
	envPath := filepath.Join(imgDir, "env.json")

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s image %s not found (use 'flect pull %s')\n",
			fail(""), imageName, imageName)
		os.Exit(1)
	}

	type ImageEnv struct {
		Path    string            `json:"path"`
		Env     map[string]string `json:"env"`
		Workdir string            `json:"workdir"`
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s reading image env: %v\n", fail(""), err)
		os.Exit(1)
	}
	var imgEnv ImageEnv
	if err := json.Unmarshal(data, &imgEnv); err != nil {
		fmt.Fprintf(os.Stderr, "%s parsing image env: %v\n", fail(""), err)
		os.Exit(1)
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	workdir := proj
	if imgEnv.Workdir != "" {
		workdir = imgEnv.Workdir
	}
	cmd.Dir = workdir

	cmd.Env = os.Environ()
	if imgEnv.Path != "" {
		cmd.Env = append(cmd.Env, "PATH="+imgEnv.Path)
	}
	for k, v := range imgEnv.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Env = append(cmd.Env,
		"FLECT_PROJECT="+proj,
		"FLECT_IMAGE="+imageName,
	)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("run error:"), err)
		os.Exit(1)
	}
}

func runShell(proj string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
		if _, err := exec.LookPath("sh"); err != nil {
			shell = "cmd.exe"
		}
	}
	runExec(proj, []string{shell})
}

func runCheck(proj string, names []string) {
	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading config:"), err)
		os.Exit(1)
	}

	if len(names) == 0 {
		for name := range cfg.Services {
			names = append(names, name)
		}
	}

	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading state:"), err)
		os.Exit(1)
	}

	allOk := true
	for _, name := range names {
		svc, ok := cfg.Services[name]
		if !ok {
			fmt.Printf("  %s %s: unknown service\n", red(cross), name)
			allOk = false
			continue
		}
		s := st.Services[name]
		if s.Running {
			healthy := true
			if svc.HealthCheck != nil {
				handles := getProcesses(name)
				for _, h := range handles {
					hcCtx, hcCancel := context.WithTimeout(context.Background(), 5*time.Second)
					if !waitHealthCheck(hcCtx, svc.HealthCheck, h.Port) {
						healthy = false
					}
					hcCancel()
				}
			}
			if healthy {
				fmt.Printf("  %s %s: healthy (PID %d, port %d)\n",
					green(check), cyanBold(name), s.PID, s.Port)
			} else {
				fmt.Printf("  %s %s: running but health check failed (PID %d)\n",
					yellow(warn), cyanBold(name), s.PID)
				allOk = false
			}
		} else {
			fmt.Printf("  %s %s: stopped\n", red(cross), cyanBold(name))
			allOk = false
		}
	}

	if !allOk {
		os.Exit(1)
	}
}
