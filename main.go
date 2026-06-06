package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	cmd := os.Args[1]
	proj, _ := os.Getwd()

	checkProject := func() {
		if !isFlectProject(proj) {
			fmt.Fprintf(os.Stderr, "flect: not a flect project (run 'flect init' first)\n")
			os.Exit(1)
		}
	}

	checkConfig := func() {
		if !hasConfig(proj) {
			fmt.Fprintf(os.Stderr, "flect: no flect.json found in project root\n")
			os.Exit(1)
		}
	}

	switch cmd {
	case "init":
		runInit(proj, os.Args[2:])

	case "start":
		checkProject()
		checkConfig()
		runStart(proj, os.Args[2:])

	case "stop":
		checkProject()
		runStop(proj, os.Args[2:])

	case "restart":
		checkProject()
		checkConfig()
		runStop(proj, os.Args[2:])
		runStart(proj, os.Args[2:])

	case "run":
		checkProject()
		runExec(proj, os.Args[2:])

	case "shell":
		checkProject()
		runShell(proj)

	case "ps":
		checkProject()
		runPs(proj)

	case "logs":
		checkProject()
		runLogs(proj, os.Args[2:])

	case "clean":
		checkProject()
		runClean(proj)

	case "status":
		checkProject()
		runStatus(proj)

	case "version", "--version", "-v":
		fmt.Println("flect v0.1.0")
		fmt.Println("universal project runtime — zero dependencies")

	case "help", "--help", "-h":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "flect: unknown command '%s'\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`flect — universal project runtime

Usage:
  flect init       Initialize a new flect project
  flect start      Start all services
  flect stop       Stop all services
  flect restart    Restart all services
  flect run <cmd>  Run a command in the project context
  flect shell      Open an interactive shell
  flect ps         List running services
  flect status     Show project status
  flect logs       Show service logs
  flect clean      Remove .flect directory (keeps source)
  flect version    Print version information
  flect help       Show this help

A flect project has a flect.json config and a .flect/ directory.
Data persists in .flect/ across stop/start cycles.`)
}
