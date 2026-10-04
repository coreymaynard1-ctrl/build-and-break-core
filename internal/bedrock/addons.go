package bedrock

import (
	"archive/tar"
	"archive/zip"
	"buildandbreak/platform/internal/httpx"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type Pack struct {
	ID      string
	Name    string
	Version []int
	Kind    string
}
type Bundle struct {
	Packs   []Pack
	Archive []byte
}
type limits struct {
	bytes int64
	files int
}

// ParseAddon accepts manifest-based packs and nested mcpack bundles. All writes
// use generated UUID directories; archive paths, links and sizes are checked.
func ParseAddon(data []byte) (Bundle, error) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	packs := []Pack{}
	budget := &limits{}
	if err := decode(data, 0, tw, &packs, budget); err != nil {
		return Bundle{}, err
	}
	if len(packs) == 0 {
		return Bundle{}, fmt.Errorf("no supported Bedrock pack manifests found")
	}
	seen := map[string]bool{}
	for _, p := range packs {
		if seen[p.ID] {
			return Bundle{}, fmt.Errorf("duplicate pack UUID")
		}
		seen[p.ID] = true
	}
	if err := tw.Close(); err != nil {
		return Bundle{}, err
	}
	return Bundle{Packs: packs, Archive: out.Bytes()}, nil
}

func safePath(name string) bool {
	return name != "" && len(name) <= 240 && !strings.ContainsAny(name, "\\\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == strings.TrimSuffix(name, "/") && name != ".." && !strings.HasPrefix(name, "../")
}

func decode(data []byte, depth int, tw *tar.Writer, packs *[]Pack, budget *limits) error {
	if depth > 2 {
		return fmt.Errorf("addon archive nesting is too deep")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid mcpack/mcaddon archive")
	}
	roots := map[string]Pack{}
	for _, f := range zr.File {
		budget.files++
		if budget.files > 2048 || !safePath(f.Name) || (f.Mode()&os.ModeType != 0 && !f.FileInfo().IsDir()) {
			return fmt.Errorf("unsafe archive path or too many entries")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if path.Base(f.Name) == "manifest.json" {
			b, err := read(f, budget, 32768)
			if err != nil {
				return err
			}
			var m struct {
				Header struct {
					UUID    string `json:"uuid"`
					Name    string `json:"name"`
					Version []int  `json:"version"`
				} `json:"header"`
				Modules []struct {
					Type    string `json:"type"`
					UUID    string `json:"uuid"`
					Version []int  `json:"version"`
				} `json:"modules"`
			}
			if json.Unmarshal(b, &m) != nil || !httpx.UUID(m.Header.UUID) || len(m.Header.Version) != 3 || len(m.Header.Name) == 0 || len(m.Header.Name) > 120 {
				return fmt.Errorf("invalid pack header")
			}
			for _, v := range m.Header.Version {
				if v < 0 || v > 1000000 {
					return fmt.Errorf("invalid pack version")
				}
			}
			kind := ""
			moduleIDs := map[string]bool{}
			for _, module := range m.Modules {
				if !httpx.UUID(module.UUID) || len(module.Version) != 3 || moduleIDs[module.UUID] || module.UUID == m.Header.UUID {
					return fmt.Errorf("invalid or duplicate pack module UUID/version")
				}
				moduleIDs[module.UUID] = true
				for _, v := range module.Version {
					if v < 0 || v > 1000000 {
						return fmt.Errorf("invalid module version")
					}
				}
				if module.Type == "data" || module.Type == "script" {
					if kind == "resource" {
						return fmt.Errorf("mixed pack modules")
					}
					kind = "behavior"
				}
				if module.Type == "resources" {
					if kind == "behavior" {
						return fmt.Errorf("mixed pack modules")
					}
					kind = "resource"
				}
			}
			if kind == "" {
				return fmt.Errorf("pack must contain behavior or resource modules")
			}
			root := path.Dir(f.Name)
			if root == "." {
				root = ""
			} else {
				root += "/"
			}
			roots[root] = Pack{ID: m.Header.UUID, Name: m.Header.Name, Version: m.Header.Version, Kind: kind}
		}
	}
	for root, p := range roots {
		*packs = append(*packs, p)
		if len(*packs) > 16 {
			return fmt.Errorf("maximum 16 packs per upload")
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, root) {
				continue
			}
			rel := strings.TrimPrefix(f.Name, root)
			// A nested manifest defines its own pack rather than a child of this one.
			nested := false
			for other := range roots {
				if other != root && strings.HasPrefix(other, root) && strings.HasPrefix(f.Name, other) {
					nested = true
				}
			}
			if nested {
				continue
			}
			b, err := read(f, budget, 64<<20)
			if err != nil {
				return err
			}
			name := p.Kind + "_packs/" + p.ID + "/" + rel
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
				return err
			}
			if _, err := tw.Write(b); err != nil {
				return err
			}
		}
	}
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".mcpack") {
			b, err := read(f, budget, 16<<20)
			if err != nil {
				return err
			}
			if err = decode(b, depth+1, tw, packs, budget); err != nil {
				return err
			}
		}
	}
	return nil
}

func read(f *zip.File, budget *limits, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, fmt.Errorf("pack file exceeds size limit")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	budget.bytes += int64(len(b))
	if err != nil || int64(len(b)) > max || budget.bytes > 60<<20 {
		return nil, fmt.Errorf("addon exceeds extraction limit")
	}
	return b, nil
}
