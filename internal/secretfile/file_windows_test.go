package secretfile

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBroadWindowsACLRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := Create(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, 256); err == nil {
		t.Fatal("Everyone-readable credential accepted")
	}
}
