package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/qiniu/ci-runner/internal/state"
)

const (
	defaultAccountListLimit = 20
	maxAccountListLimit     = 100
)

type adminAccountsResponse struct {
	Accounts         []state.AccountListItem `json:"accounts"`
	CurrentAccountID int64                   `json:"current_account_id"`
	Stats            state.AccountStats      `json:"stats"`
	Total            int64                   `json:"total"`
	Limit            int                     `json:"limit"`
	Offset           int                     `json:"offset"`
}

type adminAccountRoleRequest struct {
	Role string `json:"role"`
}

func (s *Server) handleAdminListAccounts(w http.ResponseWriter, r *http.Request) {
	_, currentAccount, ok := s.requireAdminAccount(w, r)
	if !ok {
		return
	}
	limit, offset, err := parsePagination(r, defaultAccountListLimit, maxAccountListLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	role := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if role != "" && role != "admin" && role != "user" {
		writeError(w, http.StatusBadRequest, "role must be admin or user")
		return
	}
	accounts, total, err := s.store.ListAccounts(state.AccountListOptions{
		Query:  r.URL.Query().Get("q"),
		Role:   role,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.logger.Error("list admin accounts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list accounts")
		return
	}
	stats, err := s.store.GetAccountStats()
	if err != nil {
		s.logger.Error("get admin account stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get account statistics")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writePaginationHeaders(w, r, total, limit, offset)
	writeJSON(w, http.StatusOK, adminAccountsResponse{
		Accounts:         accounts,
		CurrentAccountID: currentAccount.ID,
		Stats:            stats,
		Total:            total,
		Limit:            limit,
		Offset:           offset,
	})
}

func (s *Server) handleAdminUpdateAccountRole(w http.ResponseWriter, r *http.Request) {
	session, currentAccount, ok := s.requireAdminAccount(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeError(w, http.StatusBadRequest, "account id must be a positive integer")
		return
	}
	if accountID == currentAccount.ID {
		writeError(w, http.StatusConflict, "cannot change your own role")
		return
	}
	var input adminAccountRoleRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid account role payload")
		return
	}
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role != "admin" && role != "user" {
		writeError(w, http.StatusBadRequest, "role must be admin or user")
		return
	}
	provider := strings.TrimSpace(session.Provider)
	if provider == "" {
		provider = "github"
	}
	auditActor := provider + ":" + strings.TrimSpace(session.Subject)
	updated, err := s.store.UpdateAccountRoleWithAudit(state.AccountRoleUpdate{
		ActorAccountID: currentAccount.ID,
		AccountID:      accountID,
		Role:           role,
		AuditActor:     auditActor,
	})
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		if errors.Is(err, state.ErrConflict) {
			writeError(w, http.StatusConflict, "account roles changed; refresh and try again")
			return
		}
		s.logger.Error("update account role", "account_id", accountID, "role", role, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update account role")
		return
	}
	s.logger.Info("account role updated", "account_id", accountID, "new_role", updated.Role, "actor", auditActor, "actor_login", session.Login)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) requireAdminAccount(w http.ResponseWriter, r *http.Request) (adminSession, state.Account, bool) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return adminSession{}, state.Account{}, false
	}
	if session.Role != "admin" {
		s.logger.Warn("admin auth rejected", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "has_authorization", r.Header.Get("Authorization") != "")
		writeError(w, http.StatusUnauthorized, "missing or invalid github oauth session")
		return adminSession{}, state.Account{}, false
	}
	return session, account, true
}
