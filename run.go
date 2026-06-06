package main

import (
	"fmt"
	"os"
	"os/exec"
)

func runExec(proj string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "flect: usage: flect run <command> [args...]")
		os.Exit(1)
	}

	fullCmd := args[0]
	extraArgs := args[1:]
	parts := splitCommand(fullCmd)

	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(parts[0], append(parts[1:], extraArgs...)...)
	cmd.Dir = proj
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		"FLECT_PROJECT="+proj,
		"FLECT_RUNNING=true",
	)

	// Add service ports to env
	for name, s := range st.Services {
		if s.Running && s.Port > 0 {
			cmd.Env = append(cmd.Env,
				fmt.Sprintf("FLECT_%s_PORT=%d", name, s.Port),
			)
		}
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "flect: run error: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	if len(names) == 0 {
		for name := range cfg.Services {
			names = append(names, name)
		}
	}

	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	allOk := true
	for _, name := range names {
		svc, ok := cfg.Services[name]
		if !ok {
			fmt.Printf("flect: %s: unknown service\n", name)
			allOk = false
			continue
		}
		s := st.Services[name]
		if s.Running {
			if svc.HealthPort > 0 {
				if portInUse(svc.HealthPort) {
					fmt.Printf("flect: %s: healthy (PID %d, port %d)\n",
						name, s.PID, svc.HealthPort)
				} else {
					fmt.Printf("flect: %s: running but port %d not open (PID %d)\n",
						name, svc.HealthPort, s.PID)
					allOk = false
				}
			} else {
				fmt.Printf("flect: %s: running (PID %d)\n", name, s.PID)
			}
		} else {
			fmt.Printf("flect: %s: stopped\n", name)
			allOk = false
		}
	}

	if !allOk {
		os.Exit(1)
	}
}
