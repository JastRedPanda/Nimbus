//go:build windows

package ui

import (
	"fmt"
	"syscall"

	"github.com/JastRedPanda/Nimbus/internal/build"
	"github.com/lxn/win"
)

func ShowAbout() {
	title := syscall.StringToUTF16("About Nimbus")
	msg := syscall.StringToUTF16(fmt.Sprintf("Nimbus\nМультиплатформний інформер погоди.\n\n%s\nv%s", build.Date, build.Version))
	win.MessageBox(0, &msg[0], &title[0], win.MB_OK|win.MB_ICONINFORMATION)
}
