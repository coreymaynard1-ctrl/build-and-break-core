package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"github.com/moby/moby/client"
	"io"
	"strings"
	"time"
)

// The launcher runs after image setup drops to the game user. Docker and host
// commands are never built from customer input. The command executes inside
// the existing limited container, behind the console/signal runner.
const startupRunner = `#!/bin/bash
set -euo pipefail
runner=()
while [[ $# -gt 0 && "$1" != "--" ]]; do runner+=("$1"); shift; done
[[ $# -gt 0 ]] && shift
[[ $# -gt 0 ]] || { echo "Missing template launch command" >&2; exit 64; }
printf -v default_command '%q ' "$@"
printf '%s' "$default_command" > /data/.bnb-template-startup
if [[ "${BNB_STARTUP_COMMAND:-}" == "" || "$BNB_STARTUP_COMMAND" == '{{DEFAULT_COMMAND}}' ]]; then
 exec mc-server-runner "${runner[@]}" -- "$@"
fi
printf -v default_command '%q ' "$@"
jar=''
previous=''
for arg in "$@"; do
 if [[ "$previous" == '-jar' ]]; then jar="$arg"; break; fi
 previous="$arg"
done
printf -v quoted_jar '%q' "$jar"
command="$BNB_STARTUP_COMMAND"
command="${command//\{\{DEFAULT_COMMAND\}\}/$default_command}"
command="${command//\{\{SERVER_JARFILE\}\}/$quoted_jar}"
command="${command//\{\{SERVER_MEMORY\}\}/${BNB_HEAP_MB}}"
command="${command//\{\{SERVER_PORT\}\}/${BNB_INTERNAL_PORT}}"
exec mc-server-runner "${runner[@]}" -- bash -c "exec $command"
`

func (d *DockerRuntime) InstallStartupRunner(ctx context.Context, id, server, service string) error {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return err
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: "bnb-startup", Mode: 0755, Size: int64(len(startupRunner)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(startupRunner)); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := d.client.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/usr/local/bin", Content: &b})
	return err
}

// DefaultStartup reads the launcher-resolved command from a fixed owned path.
func (d *DockerRuntime) DefaultStartup(ctx context.Context, id, server, service string) (string, error) {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := d.client.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: "/data/.bnb-template-startup"})
	if err != nil {
		top, topErr := d.client.ContainerTop(ctx, id, client.ContainerTopOptions{Arguments: []string{"-eo", "args"}})
		if topErr != nil {
			return "", err
		}
		for _, row := range top.Processes {
			if len(row) == 1 {
				line := row[0]
				parts := strings.Fields(line)
				if len(parts) > 0 && (parts[0] == "java" || strings.HasSuffix(parts[0], "/java")) && len(line) <= 8192 {
					return line, nil
				}
			}
		}
		return "", err
	}
	defer result.Content.Close()
	tr := tar.NewReader(result.Content)
	hdr, err := tr.Next()
	if err != nil {
		return "", err
	}
	if hdr.Size > 8192 || hdr.Typeflag != tar.TypeReg {
		return "", fmt.Errorf("invalid template command file")
	}
	data, err := io.ReadAll(io.LimitReader(tr, 8193))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
