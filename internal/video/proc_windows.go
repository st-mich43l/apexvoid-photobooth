//go:build windows

package video

import (
	"os/exec"
	"syscall"
)

// hide stops a console window flashing when a GUI app starts ffmpeg.
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 /* CREATE_NO_WINDOW */}
}
