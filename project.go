package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func flectDir(proj string) string {
	return filepath.Join(proj, ".flect")
}

func statePath(proj string) string {
	return filepath.Join(flectDir(proj), "state.json")
}

func logsDir(proj string) string {
	return filepath.Join(flectDir(proj), "logs")
}

func dataDir(proj string) string {
	return filepath.Join(flectDir(proj), "data")
}

func isFlectProject(proj string) bool {
	info, err := os.Stat(flectDir(proj))
	return err == nil && info.IsDir()
}

func createStateDir(proj string) error {
	for _, d := range []string{
		flectDir(proj),
		logsDir(proj),
		dataDir(proj),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}
	return nil
}

func loadState(proj string) (*State, error) {
	data, err := os.ReadFile(statePath(proj))
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Services: make(map[string]ServiceState)}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	if st.Services == nil {
		st.Services = make(map[string]ServiceState)
	}
	return &st, nil
}

func saveState(proj string, st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	return os.WriteFile(statePath(proj), data, 0644)
}

func logPath(proj, service string) string {
	return filepath.Join(logsDir(proj), service+".log")
}

func writeLog(proj, service string, data []byte) error {
	f, err := os.OpenFile(logPath(proj, service),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
