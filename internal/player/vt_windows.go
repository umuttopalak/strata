//go:build windows

package player

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on ANSI escape handling in classic Windows consoles;
// Windows Terminal already has it on.
func enableVT(f *os.File) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
