package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func runStart(proj string, args []string) {
	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	if missing := checkRequiredExecutables(cfg); len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "flect: missing required executables:")
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  %s (install it or add to PATH)\n", m)
		}
		os.Exit(1)
	}

	st, _ := loadState(proj)
	hasRunning := false
	for name, s := range st.Services {
		if s.Running {
			fmt.Printf("flect: %s already running (PID %d)\n", name, s.PID)
			hasRunning = true
		}
	}
	if hasRunning {
		fmt.Println("flect: use 'flect restart' to restart or 'flect stop' first")
		return
	}

	order, err := topologicalSort(cfg.Services)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: dependency error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("flect: starting services...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	running := make(map[string]*ProcessHandle)
	shuttingDown := false

	go func() {
		<-sigCh
		if shuttingDown {
			return
		}
		shuttingDown = true
		fmt.Println("\nflect: shutting down...")
		for i := len(order) - 1; i >= 0; i-- {
			name := order[i]
			if h, ok := running[name]; ok && h != nil {
				svc := cfg.Services[name]
				sig := svc.StopSignal
				if sig == "" {
					sig = "SIGTERM"
				}
				stopProcess(h, sig)
				waitProcess(h, 10*time.Second)
				fmt.Printf("flect: stopped %s\n", name)
				s := st.Services[name]
				s.Running = false
				st.Services[name] = s
				saveState(proj, st)
			}
		}
		os.Exit(0)
	}()

	// Start services sequentially in dependency order
	for _, name := range order {
		svc := cfg.Services[name]

		if shuttingDown {
			break
		}

		// Wait for dependencies to be healthy
		for _, dep := range svc.DependsOn {
			if h, ok := running[dep]; ok && h != nil && h.Port > 0 {
				fmt.Printf("flect: waiting for %s (port %d)...\n", dep, h.Port)
				if !waitForPort(h.Port, 30*time.Second) {
					fmt.Fprintf(os.Stderr, "flect: %s not ready after 30s\n", dep)
				}
			}
		}

		handle, err := startProcess(proj, name, svc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flect: failed to start %s: %v\n", name, err)
			// Signal shutdown
			for i := len(order) - 1; i >= 0; i-- {
				n := order[i]
				if h, ok := running[n]; ok && h != nil {
					stopProcess(h, "SIGTERM")
					waitProcess(h, 5*time.Second)
				}
			}
			os.Exit(1)
		}

		running[name] = handle
		st.Services[name] = ServiceState{
			PID:     handle.Cmd.Process.Pid,
			Running: true,
			Port:    svc.Port,
			Started: handle.Started.Format(time.RFC3339),
		}
		saveState(proj, st)
		fmt.Printf("flect: started %s (PID %d)\n", name, handle.Cmd.Process.Pid)
	}

	// Block and wait for any service to exit
	done := make(chan struct{}, 1)
	go func() {
		for {
			for _, name := range order {
				if h, ok := running[name]; ok {
					if processExited(h) {
						fmt.Printf("flect: %s exited\n", name)
						done <- struct{}{}
						return
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		fmt.Println("flect: a service exited — shutting down all services")
		for i := len(order) - 1; i >= 0; i-- {
			name := order[i]
			if h, ok := running[name]; ok && h != nil {
				stopProcess(h, "SIGTERM")
				waitProcess(h, 5*time.Second)
			}
		}
	}
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
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
