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

	switch cmd {
	case "init":
		runInit(proj, os.Args[2:])

	case "start":
		checkProject(proj)
		checkConfig(proj)
		runStart(proj, os.Args[2:])

	case "stop":
		checkProject(proj)
		runStop(proj, os.Args[2:])

	case "restart":
		checkProject(proj)
		checkConfig(proj)
		runStop(proj, os.Args[2:])
		runStart(proj, os.Args[2:])

	case "scale":
		checkProject(proj)
		checkConfig(proj)
		runScale(proj, os.Args[2:])

	case "run":
		checkProject(proj)
		runExec(proj, os.Args[2:])

	case "shell":
		checkProject(proj)
		runShell(proj)

	case "ps":
		checkProject(proj)
		runPs(proj)

	case "status":
		checkProject(proj)
		runStatus(proj)

	case "logs":
		checkProject(proj)
		runLogs(proj, os.Args[2:])

	case "clean":
		checkProject(proj)
		runClean(proj)

	case "pull":
		runPull(os.Args[2:])

	case "images":
		runImages()

	case "version", "--version", "-v":
		fmt.Printf("flect v%s — universal project runtime\n", version)
		fmt.Println("zero dependencies, cross-platform process supervisor")

	case "help", "--help", "-h":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "%s unknown command %q\n", red(cross), cmd)
		usage()
		os.Exit(1)
	}
}

func checkProject(proj string) {
	if !isFlectProject(proj) {
		fmt.Fprintf(os.Stderr, "%s not a flect project (run 'flect init' first)\n",
			fail(""))
		os.Exit(1)
	}
}

func checkConfig(proj string) {
	if !hasConfig(proj) {
		fmt.Fprintf(os.Stderr, "%s no flect.json found in project root\n",
			fail(""))
		os.Exit(1)
	}
}

func usage() {
	fmt.Printf(`%s — universal project runtime v%s

%s
  flect init       Initialize a new flect project
  flect start      Start all services
  flect stop       Stop all services
  flect restart    Restart all services
  flect scale      Set service replica count (<service>=<n>)
  flect run        Run a command in the project context
  flect shell      Open an interactive shell
  flect ps         List running services
  flect status     Show project status
  flect logs       Show service logs
  flect clean      Remove .flect state directory
  flect pull       Download a runtime environment
  flect images     List cached runtime environments
  flect version    Print version information
  flect help       Show this help

%s
  A flect project has a flect.json config and a .flect/ directory.
  Data persists in .flect/ across stop/start cycles.
  Services are started in dependency order with health checks.
  Replicas, restart policies, and resource limits supported.
`,
		bold("flect"), version,
		bold("Usage:"),
		bold("About:"),
	)
}
