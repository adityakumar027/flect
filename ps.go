package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

func runPs(proj string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	if len(st.Services) == 0 {
		fmt.Println("flect: no services")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tPID\tPORT\tUPTIME")

	for _, name := range sortedKeys(st.Services) {
		s := st.Services[name]
		svc := cfg.Services[name]

		status := "stopped"
		pid := "-"
		port := "-"
		uptime := "-"

		if s.Running {
			status = "running"
			pid = formatPID(s.PID)
			if s.Started != "" {
				t, err := time.Parse(time.RFC3339, s.Started)
				if err == nil {
					uptime = formatDuration(time.Since(t))
				}
			}
		}
		if svc.Port > 0 {
			port = fmt.Sprintf("%d", svc.Port)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, status, pid, port, uptime)
	}
	w.Flush()
}

func runStatus(proj string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: %v\n", err)
		os.Exit(1)
	}

	running := 0
	total := len(cfg.Services)
	for _, s := range st.Services {
		if s.Running {
			running++
		}
	}

	fmt.Printf("flect: project '%s' — %d/%d services running\n",
		cfg.Project, running, total)

	if running > 0 {
		fmt.Println()
		runPs(proj)
	}
}

func runLogs(proj string, args []string) {
	serviceFilter := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		serviceFilter = args[0]
	}

	entries, err := os.ReadDir(logsDir(proj))
	if err != nil {
		fmt.Fprintf(os.Stderr, "flect: no logs available: %v\n", err)
		os.Exit(1)
	}

	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".log")
		if serviceFilter != "" && name != serviceFilter {
			continue
		}

		found = true
		data, err := os.ReadFile(logPath(proj, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "flect: error reading %s log: %v\n", name, err)
			continue
		}

		fmt.Printf("=== %s ===\n", name)
		fmt.Println(string(data))
	}

	if !found {
		if serviceFilter != "" {
			fmt.Printf("flect: no logs for '%s'\n", serviceFilter)
		} else {
			fmt.Println("flect: no logs available")
		}
	}
}

func runClean(proj string) {
	fmt.Print("flect: remove .flect directory? This will delete all data and logs. [y/N] ")
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("flect: cancelled")
		return
	}

	st, err := loadState(proj)
	if err == nil {
		for _, s := range st.Services {
			if s.Running && s.PID > 0 {
				proc, _ := os.FindProcess(s.PID)
				if proc != nil {
					proc.Kill()
				}
			}
		}
	}

	if err := os.RemoveAll(flectDir(proj)); err != nil {
		fmt.Fprintf(os.Stderr, "flect: clean error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("flect: .flect directory removed")
}

func sortedKeys(m map[string]ServiceState) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort (will be alphabetical)
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
