//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

func enableConsole() {
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

func restrictConfigPath(path string) error {
	if err := restrictToCurrentUser(path); err != nil {
		return err
	}
	return restrictToCurrentUser(filepath.Dir(path))
}

func restrictToCurrentUser(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		return err
	}
	runtime.KeepAlive(user)

	access := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
	runtime.KeepAlive(sid)
	return err
}
