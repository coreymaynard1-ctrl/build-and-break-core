package platform

import (
	"fmt"
	"regexp"
	"strings"
)

const MinecraftDefaultStartup = "{{DEFAULT_COMMAND}}"

var startupVariable = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func validateStartup(command string) error {
	if len(command) > 2048 || strings.ContainsAny(command, "\x00\r\n") || strings.TrimSpace(command) == "" {
		return fmt.Errorf("startup command must be one line, up to 2048 bytes")
	}
	remaining := command
	for _, match := range startupVariable.FindAllStringSubmatch(command, -1) {
		switch match[1] {
		case "DEFAULT_COMMAND", "SERVER_JARFILE", "SERVER_MEMORY", "SERVER_PORT":
		default:
			return fmt.Errorf("unknown startup variable %s", match[1])
		}
		remaining = strings.ReplaceAll(remaining, match[0], "")
	}
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return fmt.Errorf("invalid startup variable")
	}
	return nil
}

func displayStartup(resolved string) string {
	if resolved != "" {
		return resolved
	}
	return "java -Xms{{SERVER_MEMORY}}M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}} nogui"
}
