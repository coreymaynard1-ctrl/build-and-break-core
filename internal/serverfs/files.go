// Package serverfs implements operations confined to a server data directory.
package serverfs

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

const MaxFileBytes = 24 << 10

type Request struct {
	Operation   string `json:"operation"`
	Path        string `json:"path"`
	Destination string `json:"destination,omitempty"`
	Content     string `json:"content,omitempty"`
}
type Entry struct {
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	Symlink   bool   `json:"symlink"`
}
type Result struct {
	Entries []Entry `json:"entries,omitempty"`
	Content string  `json:"content,omitempty"`
	Path    string  `json:"path"`
}

func validPath(p string) bool {
	return len(p) > 0 && len(p) <= 1024 && !strings.ContainsAny(p, "\x00\\") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
func Validate(r Request) error {
	if !validPath(r.Path) {
		return fmt.Errorf("invalid relative file path")
	}
	switch r.Operation {
	case "list", "read":
	case "write", "mkdir", "delete":
		if r.Path == "." {
			return fmt.Errorf("cannot change server root")
		}
	case "rename":
		if r.Path == "." || !validPath(r.Destination) || r.Destination == "." {
			return fmt.Errorf("invalid rename destination")
		}
	default:
		return fmt.Errorf("unknown file operation")
	}
	if r.Operation == "write" {
		b, err := base64.StdEncoding.DecodeString(r.Content)
		if err != nil || len(b) > MaxFileBytes {
			return fmt.Errorf("file editor/upload limit is 24 KiB")
		}
	}
	return nil
}

func Run(rootPath string, r Request) (Result, error) {
	out := Result{Path: r.Path}
	if err := Validate(r); err != nil {
		return out, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return out, err
	}
	defer root.Close()
	switch r.Operation {
	case "list":
		f, err := root.Open(r.Path)
		if err != nil {
			return out, err
		}
		defer f.Close()
		entries, err := f.ReadDir(101)
		if err != nil && err != io.EOF {
			return out, err
		}
		if len(entries) > 100 {
			return out, fmt.Errorf("folder has more than 100 entries; use a narrower path")
		}
		out.Entries = []Entry{}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				return out, err
			}
			out.Entries = append(out.Entries, Entry{Name: e.Name(), Directory: e.IsDir(), Size: info.Size(), Symlink: e.Type()&os.ModeSymlink != 0})
		}
		sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	case "read":
		f, err := root.Open(r.Path)
		if err != nil {
			return out, err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
		if err != nil {
			return out, err
		}
		if len(b) > MaxFileBytes {
			return out, fmt.Errorf("file exceeds editor/download limit of 24 KiB")
		}
		out.Content = base64.StdEncoding.EncodeToString(b)
	case "write":
		data, _ := base64.StdEncoding.DecodeString(r.Content)
		f, err := root.OpenFile(r.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return out, err
		}
		_, err = f.Write(data)
		closeErr := f.Close()
		if err != nil {
			return out, err
		}
		if closeErr != nil {
			return out, closeErr
		}
	case "mkdir":
		err = root.Mkdir(r.Path, 0755)
	case "rename":
		if _, e := root.Lstat(r.Destination); e == nil {
			return out, fmt.Errorf("destination already exists")
		}
		err = root.Rename(r.Path, r.Destination)
	case "delete":
		err = root.Remove(r.Path)
	}
	return out, err
}
