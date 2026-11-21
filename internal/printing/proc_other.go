//go:build !windows

package printing

import "os/exec"

func hideWindow(*exec.Cmd) {}
