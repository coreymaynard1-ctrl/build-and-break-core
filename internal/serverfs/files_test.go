package serverfs

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesAndSymlinkConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"read", "write", "list"} {
		_, err := Run(root, Request{Operation: op, Path: "escape/private", Content: base64.StdEncoding.EncodeToString([]byte("overwrite"))})
		if err == nil {
			t.Fatalf("%s escaped root", op)
		}
	}
	if _, err := Run(root, Request{Operation: "mkdir", Path: "config"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, Request{Operation: "write", Path: "config/test.txt", Content: base64.StdEncoding.EncodeToString([]byte("preserved"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, Request{Operation: "rename", Path: "config/test.txt", Destination: "config/renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	r, err := Run(root, Request{Operation: "read", Path: "config/renamed.txt"})
	if err != nil || r.Content != base64.StdEncoding.EncodeToString([]byte("preserved")) {
		t.Fatalf("read %v", err)
	}
	if _, err = Run(root, Request{Operation: "delete", Path: "config/renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(outside, "private"))
	if string(b) != "outside" {
		t.Fatal("outside file changed")
	}
	for _, p := range []string{"../outside", "/etc/passwd", "config/../../outside", "."} {
		if Validate(Request{Operation: "delete", Path: p}) == nil {
			t.Fatalf("unsafe path %s", p)
		}
	}
}
