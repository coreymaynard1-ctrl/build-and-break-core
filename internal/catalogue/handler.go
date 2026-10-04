// Package catalogue owns the product deployment roadmap. Only implemented
// templates appear in the separate launchable-template API.
package catalogue

import (
	_ "embed"
	"net/http"
)

//go:embed games.json
var games []byte

func List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(games)
}
