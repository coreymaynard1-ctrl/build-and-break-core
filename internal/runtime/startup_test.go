package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupRunnerPreservesArgumentsAndExpandsCommand(t *testing.T) {
	dir := t.TempDir()
	runner := filepath.Join(dir, "bnb-startup")
	if err := os.WriteFile(runner, []byte(strings.ReplaceAll(startupRunner, "/data/.bnb-template-startup", filepath.Join(dir, "default-startup"))), 0755); err != nil {
		t.Fatal(err)
	}
	// Replace only the external process runner, exercising the real Bash wrapper.
	if err := os.WriteFile(filepath.Join(dir, "mc-server-runner"), []byte("#!/bin/bash\nwhile [[ $# -gt 0 && \"$1\" != -- ]]; do shift; done\nshift\nexec \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, template := range []string{"{{DEFAULT_COMMAND}}", "env BNB_TEST_HEAP={{SERVER_MEMORY}} {{DEFAULT_COMMAND}}"} {
		cmd := exec.Command(runner, "--console-in-pipe", "--", "printf", "%s|%s", "hello world", "literal $(exit 99)")
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "BNB_STARTUP_COMMAND="+template, "BNB_HEAP_MB=1536", "BNB_INTERNAL_PORT=25565")
		out, err := cmd.CombinedOutput()
		if err != nil || string(out) != "hello world|literal $(exit 99)" {
			t.Fatalf("template %s: %s %v", template, out, err)
		}
	}
	cmd := exec.Command(runner, "--", "java", "-jar", "a jar.jar")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "BNB_STARTUP_COMMAND=printf '%s|%s|%s' {{SERVER_JARFILE}} {{SERVER_MEMORY}} {{SERVER_PORT}}", "BNB_HEAP_MB=1536", "BNB_INTERNAL_PORT=25565")
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "a jar.jar|1536|25565" {
		t.Fatalf("variables: %s %v", out, err)
	}
}
