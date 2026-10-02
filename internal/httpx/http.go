package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func UUID(s string) bool { return uuidPattern.MatchString(s) }
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, code int, message string) {
	JSON(w, code, map[string]string{"error": message})
}
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		Error(w, 400, "invalid JSON body or unknown field")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		Error(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}
func AuthNode(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool) bool {
	id := r.PathValue("id")
	if !UUID(id) {
		Error(w, 400, "invalid node ID")
		return false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		Error(w, 401, "missing node credentials")
		return false
	}
	sum := sha256.Sum256([]byte(token))
	var ok bool
	err := db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM node_credentials WHERE node_id=$1 AND token_hash=$2)`, id, hex.EncodeToString(sum[:])).Scan(&ok)
	if err != nil {
		Error(w, 500, "failed to authenticate node")
		return false
	}
	if !ok {
		Error(w, 401, "invalid node credentials")
		return false
	}
	return true
}
