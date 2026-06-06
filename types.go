package main

import (
	"fmt"
	"os/exec"
	"time"
)

const version = "0.2.0"

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
)

const (
	check = "\u2714"
	cross = "\u2718"
	warn  = "\u25CF"
)

func colorize(s, color string) string {
	return color + s + ansiReset
}

func green(s string) string  { return colorize(s, ansiGreen) }
func red(s string) string    { return colorize(s, ansiRed) }
func yellow(s string) string { return colorize(s, ansiYellow) }
func cyan(s string) string   { return colorize(s, ansiCyan) }
func bold(s string) string   { return colorize(s, ansiBold) }
func cyanBold(s string) string {
	return ansiCyan + ansiBold + s + ansiReset
}

func success(format string, a ...any) string {
	return green(check+" ") + bold("flect: ") + fmt.Sprintf(format, a...)
}

func fail(format string, a ...any) string {
	return red(cross+" ") + bold("flect: ") + fmt.Sprintf(format, a...)
}

func warnMsg(format string, a ...any) string {
	return yellow(warn+" ") + bold("flect: ") + fmt.Sprintf(format, a...)
}

type ProcessHandle struct {
	Cmd      *exec.Cmd
	Service  string
	ReplicaI int
	Port     int
	Started  time.Time
}
