package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/qiniu/ci-runner/internal/state"
)

const (
	productTourOnboardingVersion   = 1
	productTourOnboardingNamespace = "onboarding"
	productTourOnboardingKey       = "product-tour"

	productTourOnboardingStatusPending   = "pending"
	productTourOnboardingStatusCompleted = "completed"
	productTourOnboardingStatusSkipped   = "skipped"
)

type productTourOnboardingState struct {
	Version  int    `json:"version"`
	Status   string `json:"status"`
	TourSeen bool   `json:"tour_seen"`
}

func (s *Server) handleUserProductTourOnboarding(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	preference, err := s.store.GetAccountPreference(
		state.AccountScopeTypeAccount,
		account.ID,
		productTourOnboardingNamespace,
		productTourOnboardingKey,
	)
	if errors.Is(err, state.ErrNotFound) {
		writeJSON(w, http.StatusOK, pendingProductTourOnboardingState())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var saved productTourOnboardingState
	if err := json.Unmarshal([]byte(preference.ValueJSON), &saved); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid stored product tour onboarding state")
		return
	}
	if saved.Version != productTourOnboardingVersion || !isSavedProductTourOnboardingState(saved) {
		saved = pendingProductTourOnboardingState()
	} else if isTerminalProductTourOnboardingStatus(saved.Status) {
		saved.TourSeen = true
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleUserSaveProductTourOnboarding(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	var input productTourOnboardingState
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid product tour onboarding payload")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid product tour onboarding payload")
		return
	}
	if input.Version != productTourOnboardingVersion || !isSavedProductTourOnboardingState(input) {
		writeError(w, http.StatusBadRequest, "invalid product tour onboarding state")
		return
	}
	if isTerminalProductTourOnboardingStatus(input.Status) {
		input.TourSeen = true
	}
	valueJSON, err := json.Marshal(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.store.UpsertAccountPreference(state.AccountPreference{
		ScopeType: state.AccountScopeTypeAccount,
		ScopeID:   account.ID,
		Namespace: productTourOnboardingNamespace,
		Key:       productTourOnboardingKey,
		ValueJSON: string(valueJSON),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, input)
}

func pendingProductTourOnboardingState() productTourOnboardingState {
	return productTourOnboardingState{
		Version: productTourOnboardingVersion,
		Status:  productTourOnboardingStatusPending,
	}
}

func isTerminalProductTourOnboardingStatus(status string) bool {
	return status == productTourOnboardingStatusCompleted || status == productTourOnboardingStatusSkipped
}

func isSavedProductTourOnboardingState(value productTourOnboardingState) bool {
	return isTerminalProductTourOnboardingStatus(value.Status) ||
		(value.Status == productTourOnboardingStatusPending && value.TourSeen)
}
