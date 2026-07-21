//go:build !windows

package tray

import "os/exec"

func configureCmd(cmd *exec.Cmd) {}
