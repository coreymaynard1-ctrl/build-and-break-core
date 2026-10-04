package runtime

import (
	"context"
	"encoding/binary"
	"github.com/moby/moby/client"
	"io"
	"strings"
	"time"
)

// Logs reads a bounded tail after checking the complete service ownership.
func (d *DockerRuntime) Logs(ctx context.Context, id, server, service string) (string, error) {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stream, err := d.client.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "100"})
	if err != nil {
		return "", err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, 64<<10))
	if err != nil {
		return "", err
	}
	return decodeLogTail(data), nil
}

func decodeLogTail(data []byte) string {
	out := make([]byte, 0, 4096)
	for len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data[4:8]))
		data = data[8:]
		if size > len(data) {
			size = len(data)
		}
		out = append(out, data[:size]...)
		data = data[size:]
	}
	if len(out) > 4096 {
		out = out[len(out)-4096:]
	}
	return strings.ReplaceAll(strings.ToValidUTF8(string(out), "�"), "\x00", "")
}
