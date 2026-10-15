//go:build windows

package console

import (
	"os"

	"golang.org/x/sys/windows"
)

func Enable() {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		handle := windows.Handle(f.Fd())
		var mode uint32
		if err := windows.GetConsoleMode(handle, &mode); err != nil {
			continue
		}
		mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_PROCESSED_OUTPUT
		_ = windows.SetConsoleMode(handle, mode)
	}
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
}
