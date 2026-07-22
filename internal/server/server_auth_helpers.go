package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/state"
)

func (s *Server) requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := s.adminSessionFromRequest(r); ok {
		return true
	}
	s.logger.Warn("admin auth rejected", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "has_authorization", r.Header.Get("Authorization") != "")
	writeError(w, http.StatusUnauthorized, "missing or invalid github oauth session")
	return false
}

func (s *Server) requireUserSession(w http.ResponseWriter, r *http.Request) (adminSession, state.Account, bool) {
	session, ok := s.sessionFromRequest(r)
	if !ok {
		s.logger.Warn("user auth rejected", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "missing or invalid github oauth session")
		return adminSession{}, state.Account{}, false
	}
	provider := strings.TrimSpace(session.Provider)
	if provider == "" {
		provider = "github"
	}
	account, _, err := s.store.GetAccountByOAuthIdentity(provider, session.Subject)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing or invalid github oauth session")
		return adminSession{}, state.Account{}, false
	}
	session.Role = account.Role
	return session, account, true
}

func (s *Server) adminSessionFromRequest(r *http.Request) (adminSession, bool) {
	session, ok := s.sessionFromRequest(r)
	if !ok || session.Role != "admin" {
		return adminSession{}, false
	}
	return session, true
}

func (s *Server) sessionFromRequest(r *http.Request) (adminSession, bool) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return adminSession{}, false
	}
	session, err := s.decodeAdminSession(cookie.Value)
	if err != nil {
		s.logger.Warn("admin session rejected", "path", r.URL.Path, "error", err)
		return adminSession{}, false
	}
	if time.Now().Unix() > session.ExpiresAt {
		return adminSession{}, false
	}
	provider := strings.TrimSpace(session.Provider)
	if provider == "" {
		provider = "github"
	}
	account, _, err := s.store.GetAccountByOAuthIdentity(provider, session.Subject)
	if err != nil {
		return adminSession{}, false
	}
	session.Role = account.Role
	return session, true
}

func (s *Server) encodeAdminSession(session adminSession) (string, error) {
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	payloadValue := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.signAdminSession(payloadValue)
	return payloadValue + "." + signature, nil
}

func (s *Server) decodeAdminSession(value string) (adminSession, error) {
	payloadValue, signature, ok := strings.Cut(value, ".")
	if !ok || payloadValue == "" || signature == "" {
		return adminSession{}, fmt.Errorf("malformed session")
	}
	want := s.signAdminSession(payloadValue)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(want)) != 1 {
		return adminSession{}, fmt.Errorf("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadValue)
	if err != nil {
		return adminSession{}, err
	}
	var session adminSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return adminSession{}, err
	}
	if strings.TrimSpace(session.Subject) == "" {
		return adminSession{}, fmt.Errorf("session missing subject")
	}
	if strings.TrimSpace(session.Role) == "" {
		return adminSession{}, fmt.Errorf("session missing role")
	}
	return session, nil
}

func (s *Server) signAdminSession(payload string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.AuthSessionSecret.Value()))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) githubOAuthRedirectURL(r *http.Request) string {
	if redirectURL := strings.TrimSpace(s.cfg.GitHubOAuthRedirectURL); redirectURL != "" {
		return redirectURL
	}
	scheme := "http"
	if requestIsSecure(r) {
		scheme = "https"
	}
	host := r.Host
	return scheme + "://" + host + "/auth/github/callback"
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clearCookie(w http.ResponseWriter, name, cookiePath string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     cookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
}

func isSandboxGone(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 404") ||
		strings.Contains(msg, "Sandbox can't be resumed") ||
		strings.Contains(msg, "SandboxNotFound")
}

func parsePagination(r *http.Request, defaultLimit, maxLimit int) (int, int, error) {
	q := r.URL.Query()
	limit := defaultLimit
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		limit = parsed
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	offset := 0
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func writePaginationHeaders(w http.ResponseWriter, r *http.Request, total int64, limit, offset int) {
	writePaginationHeadersWithinWindow(w, r, total, limit, offset, 0)
}

func writePaginationHeadersWithinWindow(w http.ResponseWriter, r *http.Request, total int64, limit, offset, maxWindow int) {
	w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	w.Header().Set("X-Limit", strconv.Itoa(limit))
	w.Header().Set("X-Offset", strconv.Itoa(offset))
	linkTotal := total
	if maxWindow > 0 && linkTotal > int64(maxWindow) {
		linkTotal = int64(maxWindow)
	}
	links := make([]string, 0, 2)
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		links = append(links, fmt.Sprintf("<%s>; rel=\"prev\"", pageURL(r, limit, prevOffset)))
	}
	nextOffset := int64(offset) + int64(limit)
	if nextOffset < linkTotal {
		nextLimit := limit
		if maxWindow > 0 && nextOffset+int64(nextLimit) > int64(maxWindow) {
			nextLimit = maxWindow - int(nextOffset)
		}
		if nextLimit > 0 {
			links = append(links, fmt.Sprintf("<%s>; rel=\"next\"", pageURL(r, nextLimit, int(nextOffset))))
		}
	}
	if len(links) > 0 {
		w.Header().Set("Link", strings.Join(links, ", "))
	}
}

func pageURL(r *http.Request, limit, offset int) string {
	q := r.URL.Query()
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	u := *r.URL
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}
