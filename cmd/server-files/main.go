package main

import (
	"buildandbreak/platform/internal/serverfs"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "one file request is required")
		os.Exit(1)
	}
	var req serverfs.Request
	if len(os.Args[1]) > 40000 || json.Unmarshal([]byte(os.Args[1]), &req) != nil {
		fmt.Fprintln(os.Stderr, "invalid file request")
		os.Exit(1)
	}
	result, err := serverfs.Run("/data", req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(1)
	}
}
