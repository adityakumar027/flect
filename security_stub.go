//go:build !linux

package main

import (
	"syscall"
)

func applyPlatformSecurity(attr *syscall.SysProcAttr, svc Service) error {
	return nil
}

func applyParentSecurity() {
}

func applyResourceLimits(svc Service) error {
	return nil
}
