//go:build !windows

package video

import "os/exec"

func hide(*exec.Cmd) {}
