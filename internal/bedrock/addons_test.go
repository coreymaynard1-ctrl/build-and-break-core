package bedrock

import (
	"archive/zip"
	"bytes"
	"testing"
)

func zipPack(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, content := range files {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const manifest = `{"format_version":2,"header":{"uuid":"11111111-1111-4111-8111-111111111111","name":"Test behavior pack","version":[1,0,0]},"modules":[{"type":"data","uuid":"22222222-2222-4222-8222-222222222222","version":[1,0,0]}]}`

func TestManifestPackAndTraversal(t *testing.T) {
	data := zipPack(t, map[string]string{"manifest.json": manifest, "functions/test.mcfunction": "say pack ready"})
	b, err := ParseAddon(data)
	if err != nil || len(b.Packs) != 1 || b.Packs[0].Kind != "behavior" || len(b.Archive) == 0 {
		t.Fatalf("pack parse %v", err)
	}
	nested := zipPack(t, map[string]string{"pack.mcpack": string(data)})
	if _, err = ParseAddon(nested); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape", "/absolute", "folder/../../escape", "bad\\path"} {
		if _, err = ParseAddon(zipPack(t, map[string]string{"manifest.json": manifest, name: "bad"})); err == nil {
			t.Fatalf("unsafe path accepted %s", name)
		}
	}
	if _, err = ParseAddon(zipPack(t, map[string]string{"manifest.json": "{}"})); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}
