package runtime

import (
	"encoding/binary"
	"testing"
)

func TestConsoleTailDemultiplexing(t *testing.T) {
	frame := func(text string) []byte {
		b := make([]byte, 8)
		b[0] = 1
		binary.BigEndian.PutUint32(b[4:8], uint32(len(text)))
		return append(b, []byte(text)...)
	}
	if got := decodeLogTail(append(frame("hello\n"), frame("ready\n")...)); got != "hello\nready\n" {
		t.Fatalf("multiplexed output %q", got)
	}
	if got := decodeLogTail(frame(string(make([]byte, 10000)))); len(got) > 4096 {
		t.Fatal("console output exceeds bound")
	}
	if got := decodeLogTail(frame("partial")[:11]); got != "par" {
		t.Fatal("bounded partial output not preserved")
	}
}
