//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSecureStateDirectoryRestrictsExistingAndFutureFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0777); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("state"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := secureStateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	future := filepath.Join(dir, "future.json")
	if err := os.WriteFile(future, []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{user.User.Sid.String(): true, "S-1-5-18": true, "S-1-5-32-544": true}
	for _, path := range []string{dir, existing, future} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil || sd == nil {
			t.Fatalf("read ACL for %s: %v", path, err)
		}
		acl, _, err := sd.DACL()
		if err != nil || acl == nil || acl.AceCount == 0 {
			t.Fatalf("missing private ACL for %s: %v", path, err)
		}
		for i := uint32(0); i < uint32(acl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(acl, i, &ace); err != nil {
				t.Fatal(err)
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !allowed[sid.String()] {
				t.Fatalf("unexpected ACL entry on %s: type=%d SID=%s", path, ace.Header.AceType, sid.String())
			}
		}
	}
}
