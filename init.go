package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInit(proj string, args []string) {
	if isFlectProject(proj) {
		fmt.Println(warnMsg("already initialized in %s", proj))
		return
	}

	cfgPath := filepath.Join(proj, "flect.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		defaultCfg := `{
  "version": "2",
  "project": "my-project",
  "services": {
    "app": {
      "command": "node index.js",
      "port": 3000,
      "replicas": 1,
      "restart": "on-failure",
      "env": {
        "NODE_ENV": "development"
      }
    }
  }
}`
		if err := os.WriteFile(cfgPath, []byte(defaultCfg), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", fail("error creating config:"), err)
			os.Exit(1)
		}
		fmt.Println(success("created flect.json — edit to configure your services"))
	}

	if err := createStateDir(proj); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", fail("error creating state directory:"), err)
		os.Exit(1)
	}

	fmt.Println(success("project initialized in %s", proj))

	if cfg, err := loadConfig(proj); err == nil {
		hadWarnings := false
		if missing := checkRequiredExecutables(cfg); len(missing) > 0 {
			fmt.Println(warnMsg("required executables not found:"))
			for _, m := range missing {
				fmt.Printf("  %s %s\n", yellow(warn), yellow(m))
			}
			hadWarnings = true
		}
		if err := validateConfig(cfg); err != nil {
			fmt.Println(warnMsg("config warning: %v", err))
			hadWarnings = true
		}
		if !hadWarnings {
			fmt.Println(green(check) + " config validated successfully")
		}
	}
}
