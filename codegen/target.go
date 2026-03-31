package codegen

import (
	"fmt"
	"runtime"
	"strings"
)

type Target string

const (
	TargetLinux   Target = "linux"
	TargetWindows Target = "windows"
)

func DefaultTarget() Target {
	if runtime.GOOS == "windows" {
		return TargetWindows
	}
	return TargetLinux
}

func ParseTarget(value string) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "native":
		return DefaultTarget(), nil
	case "linux":
		return TargetLinux, nil
	case "windows", "win", "win64":
		return TargetWindows, nil
	default:
		return "", fmt.Errorf("unsupported target %q", value)
	}
}

func (t Target) ObjectFormat() string {
	if t == TargetWindows {
		return "win64"
	}
	return "elf64"
}

func (t Target) ExecutableName(base string) string {
	if t == TargetWindows && !strings.HasSuffix(strings.ToLower(base), ".exe") {
		return base + ".exe"
	}
	return base
}
