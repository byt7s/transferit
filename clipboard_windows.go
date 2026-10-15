//go:build windows

package main

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipUser32           = windows.NewLazySystemDLL("user32.dll")
	clipKernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard    = clipUser32.NewProc("OpenClipboard")
	procCloseClipboard   = clipUser32.NewProc("CloseClipboard")
	procGetClipboardData = clipUser32.NewProc("GetClipboardData")
	procGlobalLock       = clipKernel32.NewProc("GlobalLock")
	procGlobalUnlock     = clipKernel32.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

func readClipboard() string {
	var opened bool
	for i := 0; i < 8; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			opened = true
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !opened {
		return ""
	}
	defer procCloseClipboard.Call()

	handle, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return ""
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(handle)

	text := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr)))
	return strings.TrimRight(text, "\r\n")
}
