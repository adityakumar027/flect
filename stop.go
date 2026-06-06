package main

import (
	"fmt"
	"os"
	"time"
)

func stopPIDStr(h *ProcessHandle) string {
	if h == nil || h.Cmd == nil || h.Cmd.Process == nil {
		return "stopped"
	}
	return fmt.Sprintf("%d", h.Cmd.Process.Pid)
}

func runStop(proj string, args []string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading state:"), err)
		os.Exit(1)
	}

	cfg, _ := loadConfig(proj)
	order, _ := topologicalSort(cfg.Services)

	hasRunning := false
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		s, ok := st.Services[name]
		if !ok || !s.Running {
			continue
		}
		hasRunning = true
		handles := getProcesses(name)
		if len(handles) > 0 {
			for _, h := range handles {
				svc := cfg.Services[name]
				sig := svc.StopSignal
				if sig == "" {
					sig = "SIGTERM"
				}
				stopProcess(h, sig)
				waitProcess(h, 10*time.Second)
				fmt.Printf("  %s %s[%d] stopped (PID %s)\n",
					green(check), cyanBold(name), h.ReplicaI,
					stopPIDStr(h))
			}
		} else if s.PID > 0 {
			proc, err := os.FindProcess(s.PID)
			if err == nil {
				proc.Signal(parseSignal("SIGTERM"))
				fmt.Printf("  %s %s (PID %d) signalled\n",
					green(check), cyanBold(name), s.PID)
			}
		}
		s.Running = false
		s.PID = 0
		s.Instances = make(map[string]int)
		st.Services[name] = s
	}

	if !hasRunning {
		fmt.Println(warnMsg("no running services"))
		return
	}

	saveState(proj, st)
	time.Sleep(500 * time.Millisecond)
	fmt.Println(success("all services stopped"))
}
