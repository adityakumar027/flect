package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInit(proj string, args []string) {
	// Check if already initialized
	if isFlectProject(proj) {
		fmt.Println("flect: already initialized in", proj)
		return
	}

	// Check for existing config
	cfgPath := filepath.Join(proj, "flect.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// Create a default config
		defaultCfg := `{
  "version": "1",
  "project": "my-project",
  "services": {
    "app": {
      "command": "node index.js",
      "port": 3000,
      "env": {
        "NODE_ENV": "development"
      }
    }
  }
}`
		if err := os.WriteFile(cfgPath, []byte(defaultCfg), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "flect: error creating config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("flect: created flect.json (edit to configure your services)")
	}

	// Create .flect directory
	if err := createStateDir(proj); err != nil {
		fmt.Fprintf(os.Stderr, "flect: error creating state directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("flect: initialized in", proj)

	// Load and validate config
	if cfg, err := loadConfig(proj); err == nil {
		if missing := checkRequiredExecutables(cfg); len(missing) > 0 {
			fmt.Println("flect: warning — required executables not found:")
			for _, m := range missing {
				fmt.Printf("  - %s\n", m)
			}
		}
		if err := validateConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "flect: config warning: %v\n", err)
		}
	}
}
