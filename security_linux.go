//go:build linux

package main

import (
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

const (
	prSetNoNewPrivs = 38
	prSetPdeathsig  = 1
)

func applyPlatformSecurity(attr *syscall.SysProcAttr, svc Service) error {
	attr.Pdeathsig = syscall.SIGTERM
	attr.Setpgid = true

	if svc.ReadonlyRoot {
		attr.Chroot = "/"
	}

	if svc.User != "" {
		u, err := user.Lookup(svc.User)
		if err != nil {
			return err
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		attr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}

	return nil
}

func applyParentSecurity() {
	syscall.Syscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0)
}

func applyResourceLimits(svc Service) error {
	if svc.Resources == nil {
		return nil
	}
	if svc.Resources.CPU != "" {
		v, err := parseCPULimit(svc.Resources.CPU)
		if err == nil && v > 0 {
			syscall.Setrlimit(syscall.RLIMIT_CPU,
				&syscall.Rlimit{Cur: uint64(v), Max: uint64(v)})
		}
	}
	if svc.Resources.Memory != "" {
		v, err := parseMemoryLimit(svc.Resources.Memory)
		if err == nil && v > 0 {
			syscall.Setrlimit(syscall.RLIMIT_AS,
				&syscall.Rlimit{Cur: uint64(v), Max: uint64(v)})
		}
	}
	return nil
}

func parseCPULimit(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseMemoryLimit(s string) (int64, error) {
	s = strings.ToUpper(s)
	mult := int64(1)
	if strings.HasSuffix(s, "K") {
		mult = 1024
		s = strings.TrimSuffix(s, "K")
	} else if strings.HasSuffix(s, "M") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "G") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "G")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return v * mult, nil
}
