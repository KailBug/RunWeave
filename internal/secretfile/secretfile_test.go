package secretfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateRoundTripAndNoOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "token")
	if err := Create(path, []byte("secret-canary\n")); err != nil {
		t.Fatal(err)
	}
	data, err := Read(path, 256)
	if err != nil || string(data) != "secret-canary\n" {
		t.Fatal("private read failed", err)
	}
	if err := Create(path, []byte("replacement")); err == nil {
		t.Fatal("overwrote existing credential")
	}
	data, _ = Read(path, 256)
	if string(data) != "secret-canary\n" {
		t.Fatal("credential changed")
	}
	if _, err := Read(path, 2); err == nil {
		t.Fatal("limit ignored")
	}
	if _, err := Read(filepath.Dir(path), 256); err == nil {
		t.Fatal("directory accepted")
	}
	if _, err := Read(path+"-missing", 256); err == nil {
		t.Fatal("missing accepted")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, 256); err == nil {
		t.Fatal("empty secret accepted")
	}
}
