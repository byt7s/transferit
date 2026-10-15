//go:build windows

package config

import (
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

func restrictPath(path string) error {
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
