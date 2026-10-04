package platform

import "testing"

func TestStartupValidation(t *testing.T) {
	for _, s := range []string{"{{DEFAULT_COMMAND}}", "java -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}} nogui"} {
		if err := validateStartup(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"", "echo bad\nnext", "{{UNKNOWN}}", "{{SERVER_MEMORY}"} {
		if validateStartup(s) == nil {
			t.Fatalf("invalid command accepted %q", s)
		}
	}
}
