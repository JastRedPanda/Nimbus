//go:build windows

package ui

import (
	"fmt"
	"syscall"

	"github.com/JastRedPanda/Nimbus/internal/webui"
	"github.com/lxn/win"
)

func ShowAbout() {
	title := syscall.StringToUTF16("About Nimbux")
	msg := syscall.StringToUTF16(fmt.Sprintf("Nimbux\nМультиплатформний інформер погоди.\n\n%s\nv%s", webui.BuildDate, webui.BuildVersion))
	win.MessageBox(0, &msg[0], &title[0], win.MB_OK|win.MB_ICONINFORMATION)
}
