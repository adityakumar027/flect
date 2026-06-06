package main

import (
	"fmt"
	"os"
	"time"
)

func runStop(proj string, args []string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	hasRunning := false
	for name, s := range st.Services {
		if s.Running && s.PID > 0 {
			hasRunning = true
			proc, err := os.FindProcess(s.PID)
			if err != nil {
				fmt.Printf("flect: %s (PID %d) not found\n", name, s.PID)
				s.Running = false
				st.Services[name] = s
				continue
			}
			if err := proc.Signal(os.Interrupt); err != nil {
				proc.Kill()
			}
			fmt.Printf("flect: stopped %s (PID %d)\n", name, s.PID)
			s.Running = false
			st.Services[name] = s
		}
	}

	if !hasRunning {
		fmt.Println("flect: no running services")
		return
	}

	saveState(proj, st)

	// Give processes time to exit
	time.Sleep(1 * time.Second)

	fmt.Println("flect: all services stopped")
}
