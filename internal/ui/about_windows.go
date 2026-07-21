//go:build windows

package ui

import (
	"fmt"
	"syscall"

	"github.com/lxn/win"
)

const (
	buildVersion = "0.1.4"
	buildDate    = "07.2026"
)

func ShowAbout() {
	title := syscall.StringToUTF16("About Nimbux")
	msg := syscall.StringToUTF16(fmt.Sprintf("Nimbux\nМультиплатформний інформер погоди.\n\n%s\nv%s", buildDate, buildVersion))
	win.MessageBox(0, &msg[0], &title[0], win.MB_OK|win.MB_ICONINFORMATION)
}
