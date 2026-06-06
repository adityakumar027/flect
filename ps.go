package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

func runPs(proj string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading state:"), err)
		os.Exit(1)
	}

	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading config:"), err)
		os.Exit(1)
	}

	if len(st.Services) == 0 {
		fmt.Println(warnMsg("no services"))
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		bold("SERVICE"), bold("STATUS"), bold("REPLICAS"),
		bold("PORTS"), bold("PID"), bold("UPTIME"))
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		"-------", "------", "--------",
		"-----", "---", "------")

	handles := snapshotProcesses()

	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		svc := cfg.Services[name]
		s := st.Services[name]
		instances := handles[name]

		status := red("stopped")
		replicas := fmt.Sprintf("0/%d", svc.Replicas)
		ports := "-"
		pidStr := "-"
		uptime := "-"

		if len(instances) > 0 {
			running := 0
			var portStrs []string
			var pidStrs []string
			var started time.Time
			hasStart := false

			for _, h := range instances {
				if h.Cmd != nil && h.Cmd.Process != nil && !processExited(h) {
					running++
					if h.Port > 0 {
						portStrs = append(portStrs, fmt.Sprintf("%d", h.Port))
					}
					pidStrs = append(pidStrs, fmt.Sprintf("%d", h.Cmd.Process.Pid))
					if !hasStart || h.Started.Before(started) {
						started = h.Started
						hasStart = true
					}
				}
			}

			replicas = fmt.Sprintf("%d/%d", running, svc.Replicas)
			if running > 0 {
				status = green("running")
			}
			if len(portStrs) > 0 {
				ports = strings.Join(portStrs, ",")
			}
			if len(pidStrs) > 0 {
				pidStr = strings.Join(pidStrs, ",")
			}
			if hasStart {
				uptime = formatDuration(time.Since(started))
			}
		} else if s.Running {
			count := len(s.Instances)
			if count < 1 {
				count = 1
			}
			replicas = fmt.Sprintf("%d/%d", count, svc.Replicas)
			status = green("running")
			if svc.Port > 0 {
				ports = fmt.Sprintf("%d", svc.Port)
			}
			pidStr = fmt.Sprintf("%d", s.PID)
			if s.Started != "" {
				t, err := time.Parse(time.RFC3339, s.Started)
				if err == nil {
					uptime = formatDuration(time.Since(t))
				}
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			cyanBold(name), status, replicas, ports, pidStr, uptime)
	}
	w.Flush()
}

func runStatus(proj string) {
	st, err := loadState(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading state:"), err)
		os.Exit(1)
	}

	cfg, err := loadConfig(proj)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("loading config:"), err)
		os.Exit(1)
	}

	running := 0
	total := 0
	targetReplicas := 0
	for name, svc := range cfg.Services {
		total++
		targetReplicas += svc.Replicas
		s := st.Services[name]
		if s.Running {
			running++
		}
	}

	projectName := cfg.Project
	if projectName == "" {
		projectName = "unnamed"
	}

	statusIcon := green(check)
	if running < total {
		statusIcon = yellow(warn)
	}

	actualInstances := countRunningInstances(st)

	fmt.Printf("  %s project %s — %d/%d services (%d/%d instances)\n",
		statusIcon, cyanBold(projectName), running, total,
		actualInstances, targetReplicas)

	if running > 0 {
		fmt.Println()
		runPs(proj)
	}
}

func countRunningInstances(st *State) int {
	count := 0
	for _, s := range st.Services {
		if s.Running {
			if len(s.Instances) > 0 {
				count += len(s.Instances)
			} else {
				count++
			}
		}
	}
	return count
}

func runLogs(proj string, args []string) {
	tailMode := false
	followMode := false
	serviceFilter := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tail", "-t":
			tailMode = true
		case "--follow", "-f":
			followMode = true
		case "--help":
			fmt.Println(`flect logs [options] [service]

Options:
  --tail, -t      Show only the last 50 lines
  --follow, -f    Follow log output (like tail -f)
  --help          Show this help`)
			return
		default:
			if !strings.HasPrefix(args[i], "-") {
				serviceFilter = args[i]
			}
		}
	}

	entries, err := os.ReadDir(logsDir(proj))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("reading logs:"), err)
		os.Exit(1)
	}

	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".log")
		if serviceFilter != "" && name != serviceFilter {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		if serviceFilter != "" {
			fmt.Println(warnMsg("no logs for %q", serviceFilter))
		} else {
			fmt.Println(warnMsg("no logs available"))
		}
		return
	}

	for _, name := range names {
		if !followMode {
			data, err := os.ReadFile(logPath(proj, name))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s reading %s: %v\n",
					fail(""), name, err)
				continue
			}
			content := string(data)
			if tailMode {
				lines := strings.Split(content, "\n")
				if len(lines) > 50 {
					lines = lines[len(lines)-50:]
				}
				content = strings.Join(lines, "\n")
			}
			if len(names) > 1 {
				fmt.Printf("=== %s ===\n", name)
			}
			fmt.Print(content)
			if !strings.HasSuffix(content, "\n") {
				fmt.Println()
			}
		} else {
			followLog(proj, name)
		}
	}
}

func followLog(proj, name string) {
	f, err := os.Open(logPath(proj, name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("following log:"), err)
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			fmt.Print(string(buf[:n]))
		}
		if err != nil {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func runClean(proj string) {
	fmt.Printf("  %s remove .flect directory? This will delete all data and logs. [y/N] ",
		yellow(warn))
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println(warnMsg("cancelled"))
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
		handles := snapshotProcesses()
		for _, instances := range handles {
			for _, h := range instances {
				stopProcess(h, "SIGKILL")
			}
		}
	}

	if err := os.RemoveAll(flectDir(proj)); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("clean error:"), err)
		os.Exit(1)
	}

	fmt.Println(success(".flect directory removed"))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours() / 24)
	h := int(d.Hours()) % 24
	if h > 0 {
		return fmt.Sprintf("%dd%dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}
