package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Version  string             `json:"version"`
	Project  string             `json:"project"`
	Services map[string]Service `json:"services"`
}

type HealthCheck struct {
	Method   string `json:"method"`
	Path     string `json:"path,omitempty"`
	Port     int    `json:"port,omitempty"`
	Command  string `json:"command,omitempty"`
	Interval int    `json:"interval,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
	Retries  int    `json:"retries,omitempty"`
}

type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type Service struct {
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	Workdir      string            `json:"workdir,omitempty"`
	Port         int               `json:"port,omitempty"`
	DependsOn    []string          `json:"depends_on,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	DataDirs     []string          `json:"data_dirs,omitempty"`
	StopSignal   string            `json:"stop_signal,omitempty"`
	User         string            `json:"user,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	ReadonlyRoot bool              `json:"readonly_root,omitempty"`
	AllowedPaths []string          `json:"allowed_paths,omitempty"`
	Resources    *Resources        `json:"resources,omitempty"`
	Replicas     int               `json:"replicas,omitempty"`
	Restart      string            `json:"restart,omitempty"`
	HealthCheck  *HealthCheck      `json:"health_check,omitempty"`
	HealthCmd    string            `json:"health_cmd,omitempty"`
	HealthPort   int               `json:"health_port,omitempty"`
}

type State struct {
	Services map[string]ServiceState `json:"services"`
}

type ServiceState struct {
	PID       int               `json:"pid,omitempty"`
	Running   bool              `json:"running"`
	Port      int               `json:"port,omitempty"`
	Started   string            `json:"started,omitempty"`
	Replicas  int               `json:"replicas,omitempty"`
	Instances map[string]int    `json:"instances,omitempty"`
}

func configPath(proj string) string {
	return filepath.Join(proj, "flect.json")
}

func hasConfig(proj string) bool {
	_, err := os.Stat(configPath(proj))
	return err == nil
}

func loadConfig(proj string) (*Config, error) {
	data, err := os.ReadFile(configPath(proj))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	migrateConfig(&cfg)
	if cfg.Services == nil {
		cfg.Services = make(map[string]Service)
	}
	for name, svc := range cfg.Services {
		if svc.Replicas < 1 {
			svc.Replicas = 1
		}
		if svc.Restart == "" {
			svc.Restart = "on-failure"
		}
		if svc.Env == nil {
			svc.Env = make(map[string]string)
		}
		cfg.Services[name] = svc
	}
	return &cfg, nil
}

func migrateConfig(cfg *Config) {
	if cfg.Version == "" || cfg.Version == "1" {
		for name, svc := range cfg.Services {
			if svc.HealthCmd != "" || svc.HealthPort != 0 {
				svc.HealthCheck = &HealthCheck{}
				if svc.HealthCmd != "" {
					svc.HealthCheck.Method = "exec"
					svc.HealthCheck.Command = svc.HealthCmd
				} else {
					svc.HealthCheck.Method = "tcp_connect"
					svc.HealthCheck.Port = svc.HealthPort
				}
				svc.HealthCmd = ""
				svc.HealthPort = 0
			}
			cfg.Services[name] = svc
		}
		cfg.Version = "2"
	}
}

func saveConfig(proj string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return os.WriteFile(configPath(proj), data, 0644)
}

func validateConfig(cfg *Config) error {
	if len(cfg.Services) == 0 {
		return fmt.Errorf("no services defined")
	}
	for name, svc := range cfg.Services {
		if svc.Command == "" {
			return fmt.Errorf("service '%s': command is required", name)
		}
	}
	for name, svc := range cfg.Services {
		for _, dep := range svc.DependsOn {
			if _, ok := cfg.Services[dep]; !ok {
				return fmt.Errorf("service '%s': depends on unknown service '%s'", name, dep)
			}
		}
	}
	_, err := topologicalSort(cfg.Services)
	if err != nil {
		return fmt.Errorf("dependency cycle: %w", err)
	}
	return nil
}

func topologicalSort(services map[string]Service) ([]string, error) {
	type entry struct {
		visited bool
		mark    bool
	}
	marker := make(map[string]*entry)
	for name := range services {
		marker[name] = &entry{}
	}
	var order []string

	var dfs func(string) error
	dfs = func(name string) error {
		e := marker[name]
		if e.mark {
			return fmt.Errorf("cycle detected at '%s'", name)
		}
		if e.visited {
			return nil
		}
		e.mark = true
		for _, dep := range services[name].DependsOn {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		e.mark = false
		e.visited = true
		order = append(order, name)
		return nil
	}

	for name := range services {
		if !marker[name].visited {
			if err := dfs(name); err != nil {
				return nil, err
			}
		}
	}
	return order, nil
}
