package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func runStart(proj string, args []string) {
	applyParentSecurity()

	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading config:"), err)
		os.Exit(1)
	}

	if missing := checkRequiredExecutables(cfg); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", fail("missing required executables:"))
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  %s %s (install or add to PATH)\n", yellow(warn), m)
		}
		os.Exit(1)
	}

	st, _ := loadState(proj)
	hasRunning := false
	for name, s := range st.Services {
		if s.Running {
			fmt.Println(warnMsg("%s already running", cyanBold(name)))
			hasRunning = true
		}
	}
	if hasRunning {
		fmt.Println(warnMsg("use 'flect restart' or 'flect stop' first"))
		return
	}

	order, err := topologicalSort(cfg.Services)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("dependency error:"), err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		select {
		case <-sigCh:
			fmt.Println()
			cancel()
		case <-ctx.Done():
		}
	}()

	fmt.Println(success("starting services..."))

	for _, name := range order {
		if ctx.Err() != nil {
			break
		}
		svc := cfg.Services[name]

		fmt.Printf("  %s %s (%d replicas)\n",
			cyan("●"), cyanBold(name), svc.Replicas)

		for _, dep := range svc.DependsOn {
			if ctx.Err() != nil {
				break
			}
			handles := getProcesses(dep)
			if len(handles) == 0 {
				fmt.Fprintf(os.Stderr, "  %s %s dependency %s not running\n",
					yellow(warn), cyanBold(name), dep)
				continue
			}
			depSvc := cfg.Services[dep]
			for _, h := range handles {
				checkCtx, checkCancel := context.WithTimeout(ctx, time.Duration(depSvc.HealthCheck.Timeout)*time.Second)
				if depSvc.HealthCheck != nil {
					fmt.Printf("  waiting for %s health check...\n", cyanBold(dep))
					if !waitHealthCheck(checkCtx, depSvc.HealthCheck, h.Port) {
						fmt.Fprintf(os.Stderr, "  %s %s health check failed for %s\n",
							red(cross), cyanBold(name), dep)
					}
				} else if h.Port > 0 {
					waitForPortContext(ctx, h.Port, 30*time.Second)
				}
				checkCancel()
			}
		}

		numReplicas := svc.Replicas
		if numReplicas < 1 {
			numReplicas = 1
		}

		discoveryEnv := buildDiscoveryEnv(cfg, order)

		type result struct {
			idx    int
			handle *ProcessHandle
			err    error
		}
		results := make(chan result, numReplicas)
		var wg sync.WaitGroup

		for i := 0; i < numReplicas; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				h, err := startProcess(proj, name, svc, idx, discoveryEnv)
				results <- result{idx, h, err}
			}(i)
		}
		wg.Wait()
		close(results)

		for r := range results {
			if r.err != nil {
				fmt.Fprintf(os.Stderr, "  %s %s[%d]: %v\n",
					red(cross), cyanBold(name), r.idx, r.err)
				cancel()
				break
			}
			addProcess(name, r.handle)
			portStr := "-"
			if r.handle.Port > 0 {
				portStr = fmt.Sprintf("%d", r.handle.Port)
			}
			fmt.Printf("  %s %s[%d] started (PID %d, port %s)\n",
				green(check), cyanBold(name), r.idx,
				r.handle.Cmd.Process.Pid, portStr)
			updateState(proj, name, r.handle)
		}

		if ctx.Err() != nil {
			break
		}

		if svc.HealthCheck != nil {
			handles := getProcesses(name)
			for _, h := range handles {
				checkCtx, checkCancel := context.WithTimeout(ctx,
					time.Duration(svc.HealthCheck.Timeout)*time.Second)
				if waitHealthCheck(checkCtx, svc.HealthCheck, h.Port) {
					fmt.Printf("  %s %s health check passed\n",
						green(check), cyanBold(name))
				} else {
					fmt.Fprintf(os.Stderr, "  %s %s health check failed\n",
						red(cross), cyanBold(name))
				}
				checkCancel()
			}
		}
	}

	if ctx.Err() != nil {
		shutdownAll(proj, cfg, order)
		os.Exit(1)
	}

	fmt.Println(success("all services started"))

	go monitorServices(ctx, proj, cfg)

	<-ctx.Done()
	shutdownAll(proj, cfg, order)
}

func buildDiscoveryEnv(cfg *Config, order []string) []string {
	allHandles := snapshotProcesses()
	var env []string
	for _, name := range order {
		handles := allHandles[name]
		if len(handles) == 0 {
			continue
		}
		upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		var hosts, ports []string
		for _, h := range handles {
			hosts = append(hosts, "127.0.0.1")
			ports = append(ports, fmt.Sprintf("%d", h.Port))
		}
		env = append(env,
			fmt.Sprintf("FLECT_%s_HOSTS=%s", upper, strings.Join(hosts, ";")),
			fmt.Sprintf("FLECT_%s_PORTS=%s", upper, strings.Join(ports, ";")),
		)
		if len(handles) == 1 {
			env = append(env,
				fmt.Sprintf("FLECT_%s_HOST=%s", upper, "127.0.0.1"),
				fmt.Sprintf("FLECT_%s_PORT=%d", upper, handles[0].Port),
			)
		}
	}
	return env
}

func waitHealthCheck(ctx context.Context, hc *HealthCheck, port int) bool {
	if hc == nil {
		return true
	}
	retries := hc.Retries
	if retries < 1 {
		retries = 3
	}
	interval := hc.Interval
	if interval < 1 {
		interval = 2
	}

	checkPort := hc.Port
	if checkPort <= 0 {
		checkPort = port
	}

	for i := 0; i < retries; i++ {
		if ctx.Err() != nil {
			return false
		}

		ok := false
		switch hc.Method {
		case "http_get":
			path := hc.Path
			if path == "" {
				path = "/"
			}
			ok = httpHealthCheck(checkPort, path)
		case "tcp_connect":
			ok = tcpHealthCheck(checkPort)
		case "exec":
			if hc.Command != "" {
				ok = execHealthCheck(hc.Command)
			}
		default:
			ok = tcpHealthCheck(checkPort)
		}

		if ok {
			return true
		}
		if i < retries-1 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(interval) * time.Second):
			}
		}
	}
	return false
}

func httpHealthCheck(port int, path string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func tcpHealthCheck(port int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func execHealthCheck(command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	parts := splitCommand(command)
	if len(parts) == 0 {
		return false
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	return cmd.Run() == nil
}

func waitForPortContext(ctx context.Context, port int, timeout time.Duration) bool {
	if port <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if portInUse(port) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func updateState(proj, name string, handle *ProcessHandle) {
	st, _ := loadState(proj)
	s := st.Services[name]
	if handle.Cmd != nil && handle.Cmd.Process != nil {
		s.PID = handle.Cmd.Process.Pid
	}
	s.Running = true
	s.Port = handle.Port
	s.Started = handle.Started.Format(time.RFC3339)
	if s.Instances == nil {
		s.Instances = make(map[string]int)
	}
	key := fmt.Sprintf("%d", handle.ReplicaI)
	s.Instances[key] = s.PID
	st.Services[name] = s
	saveState(proj, st)
}

func shutdownAll(proj string, cfg *Config, order []string) {
	fmt.Println()
	fmt.Println(warnMsg("shutting down..."))
	st, _ := loadState(proj)

	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		svc := cfg.Services[name]
		sig := svc.StopSignal
		if sig == "" {
			sig = "SIGTERM"
		}

		handles := getProcesses(name)
		for _, h := range handles {
			stopProcess(h, sig)
			waitProcess(h, 10*time.Second)
			fmt.Printf("  %s %s[%d] stopped\n",
				green(check), cyanBold(name), h.ReplicaI)
		}

		s := st.Services[name]
		s.Running = false
		s.PID = 0
		s.Instances = make(map[string]int)
		st.Services[name] = s
	}
	saveState(proj, st)
	fmt.Println(success("all services stopped"))
}

func monitorServices(ctx context.Context, proj string, cfg *Config) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(2 * time.Second)
		}

		for name, handles := range snapshotProcesses() {
			if ctx.Err() != nil {
				return
			}
			svc := cfg.Services[name]
			if svc.Restart == "never" {
				continue
			}
			for _, h := range handles {
				if processExited(h) {
					policy := svc.Restart
					if policy == "always" || policy == "on-failure" {
						fmt.Printf("  %s %s[%d] crashed — restarting (%s)\n",
							yellow(warn), cyanBold(name), h.ReplicaI, policy)
						removeProcess(name, h.ReplicaI)

						order, _ := topologicalSort(cfg.Services)
						discoveryEnv := buildDiscoveryEnv(cfg, order)

						newHandle, err := startProcess(proj, name, svc, h.ReplicaI, discoveryEnv)
						if err != nil {
							fmt.Fprintf(os.Stderr, "  %s %s[%d] restart failed: %v\n",
								red(cross), cyanBold(name), h.ReplicaI, err)
							if policy == "on-failure" {
								continue
							}
						} else {
							addProcess(name, newHandle)
							updateState(proj, name, newHandle)
							fmt.Printf("  %s %s[%d] restarted (PID %d)\n",
								green(check), cyanBold(name), h.ReplicaI,
								newHandle.Cmd.Process.Pid)
						}
					}
				}
			}
		}
	}
}

func runScale(proj string, args []string) {
	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading config:"), err)
		os.Exit(1)
	}

	if len(args) == 0 {
		fmt.Printf("  %s current replica counts:\n", cyan("●"))
		for name, svc := range cfg.Services {
			handles := getProcesses(name)
			running := 0
			for _, h := range handles {
				if !processExited(h) {
					running++
				}
			}
			fmt.Printf("    %s configured=%d running=%d\n",
				cyanBold(name), svc.Replicas, running)
		}
		return
	}

	order, _ := topologicalSort(cfg.Services)

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "%s invalid format %q (use <service>=<n>)\n",
				fail("scale:"), arg)
			continue
		}
		name := parts[0]
		var n int
		if _, err := fmt.Sscanf(parts[1], "%d", &n); err != nil || n < 0 {
			fmt.Fprintf(os.Stderr, "%s invalid count for %s\n", fail("scale:"), name)
			continue
		}

		svc, ok := cfg.Services[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "%s unknown service: %s\n", fail("scale:"), name)
			continue
		}

		old := svc.Replicas
		svc.Replicas = n
		cfg.Services[name] = svc

		if err := saveConfig(proj, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "%s saving config: %v\n", fail("scale:"), err)
			continue
		}

		if n > old {
			discoveryEnv := buildDiscoveryEnv(cfg, order)
			for i := old; i < n; i++ {
				handle, err := startProcess(proj, name, svc, i, discoveryEnv)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %s %s[%d] start failed: %v\n",
						red(cross), cyanBold(name), i, err)
				} else {
					addProcess(name, handle)
					updateState(proj, name, handle)
					fmt.Printf("  %s %s[%d] started (PID %d)\n",
						green(check), cyanBold(name), i, handle.Cmd.Process.Pid)
				}
			}
		} else if n < old {
			handles := getProcesses(name)
			for i := n; i < old; i++ {
				for _, h := range handles {
					if h.ReplicaI == i {
						stopProcess(h, "SIGTERM")
						waitProcess(h, 10*time.Second)
						removeProcess(name, i)
						fmt.Printf("  %s %s[%d] stopped\n",
							green(check), cyanBold(name), i)
						break
					}
				}
			}
		}
		fmt.Printf("  %s %s scaled %d → %d\n",
			green(check), cyanBold(name), old, n)
	}
}
