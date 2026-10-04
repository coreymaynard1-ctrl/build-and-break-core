package accounts

import (
	"buildandbreak/platform/internal/httpx"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}
type contextKey struct{}

func Current(ctx context.Context) User { u, _ := ctx.Value(contextKey{}).(User); return u }
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}
func Owner(ctx context.Context) any {
	if u := Current(ctx); u.ID != "" {
		return u.ID
	}
	return nil
}
func Scope(ctx context.Context) any {
	if IsAdmin(ctx) {
		return nil
	}
	return Owner(ctx)
}

// Direct handler tests are trusted callers. Production routes always use Protect.
func IsAdmin(ctx context.Context) bool { u := Current(ctx); return u.Role == "admin" }

type Handler struct {
	DB            *pgxpool.Pool
	SecureCookies bool
	AllowedOrigin string
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT id,email,name,role FROM accounts ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		httpx.Error(w, 500, "failed to list accounts")
		return
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role) != nil {
			httpx.Error(w, 500, "failed to read accounts")
			return
		}
		users = append(users, u)
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read accounts")
		return
	}
	httpx.JSON(w, 200, users)
}

const cookieName = "bnb_session"

func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func randomToken() string { return rand.Text() }
func (h *Handler) cookie(w http.ResponseWriter, token string, expiry time.Time) {
	age := int(time.Until(expiry).Seconds())
	if token == "" {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: age})
}
func (h *Handler) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		httpx.Error(w, 403, "cross-site request rejected")
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil || h.SecureCookies {
		scheme = "https"
	}
	parsed, err := url.Parse(origin)
	allowed := origin == scheme+"://"+r.Host
	for _, configured := range strings.Split(h.AllowedOrigin, ",") {
		if origin == strings.TrimSpace(configured) {
			allowed = true
		}
	}
	if err != nil || !allowed || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		httpx.Error(w, 403, "cross-site request rejected")
		return false
	}
	return true
}
func (h *Handler) limited(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	var attempts int
	err = h.DB.QueryRow(r.Context(), `INSERT INTO auth_rate_limits(bucket,attempts,expires_at) VALUES($1,1,NOW()+INTERVAL '15 minutes') ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN auth_rate_limits.expires_at<=NOW() THEN 1 ELSE auth_rate_limits.attempts+1 END,expires_at=CASE WHEN auth_rate_limits.expires_at<=NOW() THEN NOW()+INTERVAL '15 minutes' ELSE auth_rate_limits.expires_at END RETURNING attempts`, digest(ip)).Scan(&attempts)
	if err != nil {
		httpx.Error(w, 503, "authentication unavailable")
		return true
	}
	if attempts > 30 {
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, 429, "too many authentication attempts; try again later")
		return true
	}
	return false
}
func (h *Handler) session(w http.ResponseWriter, r *http.Request, u User) {
	token, csrf := randomToken(), randomToken()
	expires := time.Now().Add(24 * time.Hour)
	_, err := h.DB.Exec(r.Context(), `INSERT INTO account_sessions(token_hash,account_id,csrf_token,expires_at) VALUES($1,$2,$3,$4)`, digest(token), u.ID, csrf, expires)
	if err != nil {
		httpx.Error(w, 500, "failed to create session")
		return
	}
	h.cookie(w, token, expires)
	httpx.JSON(w, 200, map[string]any{"user": u, "csrf_token": csrf})
}
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.sameOrigin(w, r) || h.limited(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email || len(req.Email) > 254 || req.Name == "" || len(req.Name) > 100 || strings.ContainsAny(req.Name, "\x00\n\r") || len(req.Password) < 12 || len(req.Password) > 256 {
		httpx.Error(w, 400, "valid email, name, and password of 12–256 bytes required")
		return
	}
	password, err := hashPassword(req.Password)
	if err != nil {
		httpx.Error(w, 500, "failed to secure password")
		return
	}
	var u User
	err = h.DB.QueryRow(r.Context(), `INSERT INTO accounts(email,name,password_hash) VALUES($1,$2,$3) RETURNING id,email,name,role`, req.Email, req.Name, password).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && p.Code == "23505" {
			httpx.Error(w, 409, "account already exists; sign in instead")
		} else {
			httpx.Error(w, 500, "failed to create account")
		}
		return
	}
	h.session(w, r, u)
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.sameOrigin(w, r) || h.limited(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	if len(req.Email) > 254 || len(req.Password) > 256 {
		httpx.Error(w, 401, "invalid email or password")
		return
	}
	var u User
	var encoded string
	err := h.DB.QueryRow(r.Context(), `SELECT id,email,name,role,password_hash FROM accounts WHERE email=$1 AND NOT disabled`, strings.ToLower(strings.TrimSpace(req.Email))).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &encoded)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 503, "authentication unavailable")
		return
	}
	// Missing accounts incur the same password derivation cost.
	if err != nil {
		encoded = "pbkdf2-sha256-600000$00000000000000000000000000000000$0000000000000000000000000000000000000000000000000000000000000000"
	}
	valid := checkPassword(req.Password, encoded)
	if err != nil || !valid {
		httpx.Error(w, 401, "invalid email or password")
		return
	}
	h.session(w, r, u)
}
func (h *Handler) authenticate(r *http.Request) (User, string, error) {
	var u User
	var csrf string
	cookie, err := r.Cookie(cookieName)
	if err != nil || len(cookie.Value) > 128 {
		return u, csrf, pgx.ErrNoRows
	}
	err = h.DB.QueryRow(r.Context(), `SELECT a.id,a.email,a.name,a.role,s.csrf_token FROM account_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>NOW() AND NOT a.disabled`, digest(cookie.Value)).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &csrf)
	return u, csrf, err
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	u, csrf, err := h.authenticate(r)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.JSON(w, 200, map[string]any{"user": nil})
		return
	}
	if err != nil {
		httpx.Error(w, 503, "authentication unavailable")
		return
	}
	httpx.JSON(w, 200, map[string]any{"user": u, "csrf_token": csrf})
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(cookieName)
	if cookie != nil {
		if _, err := h.DB.Exec(r.Context(), `DELETE FROM account_sessions WHERE token_hash=$1`, digest(cookie.Value)); err != nil {
			httpx.Error(w, 500, "failed to sign out")
			return
		}
	}
	h.cookie(w, "", time.Unix(1, 0))
	httpx.JSON(w, 200, map[string]bool{"signed_out": true})
}
func (h *Handler) Protect(admin bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		u, csrf, err := h.authenticate(r)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, 401, "sign in to continue")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "authentication unavailable")
			return
		}
		if admin && u.Role != "admin" {
			httpx.Error(w, 403, "administrator access required")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !h.sameOrigin(w, r) {
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
				httpx.Error(w, 403, "invalid CSRF token")
				return
			}
		}
		next(w, r.WithContext(WithUser(r.Context(), u)))
	}
}
