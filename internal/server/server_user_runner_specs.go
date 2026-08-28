package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
)

type userRunnerSpecResponse struct {
	Name                    string   `json:"name"`
	Source                  string   `json:"source"`
	WorkflowLabels          []string `json:"workflow_labels"`
	TemplateID              string   `json:"template_id,omitempty"`
	DefaultTemplateName     string   `json:"default_template_name,omitempty"`
	RunnerGroup             string   `json:"runner_group,omitempty"`
	Enabled                 bool     `json:"enabled"`
	GlobalMaxConcurrency    int      `json:"global_max_concurrency"`
	ScopeMaxConcurrency     int      `json:"scope_max_concurrency"`
	EffectiveMaxConcurrency int      `json:"effective_max_concurrency"`
	OverridesGlobal         bool     `json:"overrides_global"`
	Editable                bool     `json:"editable"`
	ScopeControlConfigured  bool     `json:"scope_control_configured"`
	UpdatedAt               string   `json:"updated_at"`
}

type userRunnerSpecListResponse struct {
	ScopeType     string                   `json:"scope_type"`
	ScopeID       int64                    `json:"scope_id"`
	SandboxSource string                   `json:"sandbox_source"`
	SandboxRegion string                   `json:"sandbox_region,omitempty"`
	Items         []userRunnerSpecResponse `json:"items"`
}

type userRunnerSpecMutationRequest struct {
	Name              string   `json:"name"`
	WorkflowLabels    []string `json:"workflow_labels"`
	TemplateID        string   `json:"template_id"`
	RunnerGroup       string   `json:"runner_group"`
	MaxConcurrency    int      `json:"max_concurrency"`
	Enabled           bool     `json:"enabled"`
	ExpectedUpdatedAt string   `json:"expected_updated_at"`
}

type userRunnerSpecPatchRequest struct {
	WorkflowLabels    *[]string `json:"workflow_labels"`
	TemplateID        *string   `json:"template_id"`
	RunnerGroup       *string   `json:"runner_group"`
	MaxConcurrency    *int      `json:"max_concurrency"`
	Enabled           *bool     `json:"enabled"`
	ExpectedUpdatedAt string    `json:"expected_updated_at"`
}

type userRunnerSpecControlRequest struct {
	Enabled           bool   `json:"enabled"`
	MaxConcurrency    int    `json:"max_concurrency"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

func (s *Server) userRunnerScope(w http.ResponseWriter, r *http.Request, accountID int64) (state.RunnerProfileScope, accountPreferenceScope, bool) {
	prefScope, err := s.accountPreferenceScopeFromRequest(accountID, r)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "runner_spec_scope_invalid", err.Error())
		return state.RunnerProfileScope{}, accountPreferenceScope{}, false
	}
	manageable, err := s.accountPreferenceScopeManageable(r.Context(), accountID, prefScope)
	if err != nil {
		s.writeUserRepositoryAuthorizationError(w, err)
		return state.RunnerProfileScope{}, accountPreferenceScope{}, false
	}
	if !manageable {
		writeErrorCode(w, http.StatusForbidden, "runner_spec_scope_forbidden", "Runner types for this scope are managed by its owner")
		return state.RunnerProfileScope{}, accountPreferenceScope{}, false
	}
	return state.RunnerProfileScope{Type: prefScope.Type, ID: prefScope.ID}, prefScope, true
}

func (s *Server) handleUserListRunnerSpecs(w http.ResponseWriter, r *http.Request) {
	_, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, prefScope, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	items, err := s.store.ListEffectiveProfiles(scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runner types")
		return
	}
	sandboxSource := "none"
	sandboxRegion := ""
	if _, snapshot, err := s.sandboxServiceForScope(prefScope); err == nil {
		sandboxSource = snapshot.Source
		sandboxRegion = sandboxRegionForAPIURL(snapshot.APIURL)
	}
	response := userRunnerSpecListResponse{ScopeType: scope.Type, ScopeID: scope.ID, SandboxSource: sandboxSource, SandboxRegion: sandboxRegion, Items: make([]userRunnerSpecResponse, 0, len(items))}
	for _, item := range items {
		max := effectiveRunnerConcurrencyLimit(item.GlobalMaxConcurrency, item.ScopeMaxConcurrency)
		responseItem := userRunnerSpecResponse{Name: item.Profile.Name, Source: item.Source, WorkflowLabels: append([]string(nil), item.WorkflowLabels...), DefaultTemplateName: item.Profile.DefaultTemplateName, Enabled: item.EffectiveEnabled, GlobalMaxConcurrency: item.GlobalMaxConcurrency, ScopeMaxConcurrency: item.ScopeMaxConcurrency, EffectiveMaxConcurrency: max, OverridesGlobal: item.OverridesGlobal, Editable: item.Editable, ScopeControlConfigured: item.ScopeControlConfigured, UpdatedAt: item.Profile.UpdatedAt.UTC().Format(time.RFC3339Nano)}
		if item.Source == "scoped_custom" {
			responseItem.TemplateID = item.Profile.TemplateID
			responseItem.RunnerGroup = item.Profile.RunnerGroup
		}
		response.Items = append(response.Items, responseItem)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleUserPutRunnerSpecControl(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, _, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	var input userRunnerSpecControlRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.MaxConcurrency < 0 {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", "invalid runner type control")
		return
	}
	expected, ok := parseRunnerSpecRevision(w, input.ExpectedUpdatedAt, true)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if _, err := s.store.GetProfile(name); err != nil {
		writeErrorCode(w, http.StatusNotFound, "runner_spec_not_found", "runner type not found")
		return
	}
	control := state.RunnerProfileControl{ScopeType: scope.Type, ScopeID: scope.ID, ProfileName: name, Enabled: input.Enabled, MaxConcurrency: input.MaxConcurrency}
	err := s.applyMutationWithAudit("github:"+session.Subject, "user_runner_spec_control.upsert", "runner_profile_scope_control", fmt.Sprintf("%s:%d:%s", scope.Type, scope.ID, name), map[string]any{"enabled": input.Enabled, "max_concurrency": input.MaxConcurrency}, func(tx state.Store) error {
		_, err := tx.UpsertProfileControlIfUnchanged(control, &expected)
		return err
	})
	if err != nil {
		if errors.Is(err, state.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_conflict", "Runner type changed while saving; refresh and try again")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleUserListRunnerSpecs(w, r)
}

func (s *Server) handleUserDeleteRunnerSpecControl(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, _, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	expected, ok := parseRunnerSpecRevision(w, r.URL.Query().Get("expected_updated_at"), true)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	err := s.applyMutationWithAudit("github:"+session.Subject, "user_runner_spec_control.delete", "runner_profile_scope_control", fmt.Sprintf("%s:%d:%s", scope.Type, scope.ID, name), nil, func(tx state.Store) error {
		return tx.DeleteProfileControlIfUnchanged(scope, name, &expected)
	})
	if err != nil {
		if errors.Is(err, state.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_conflict", "Runner type control changed while saving; refresh and try again")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleUserListRunnerSpecs(w, r)
}

func (s *Server) handleUserCreateRunnerSpec(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, prefScope, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	var input userRunnerSpecMutationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", "invalid runner type payload")
		return
	}
	if scope.Type == state.AccountScopeTypeAccount && strings.TrimSpace(input.RunnerGroup) != "" {
		writeErrorCode(w, http.StatusBadRequest, "runner_group_not_supported", "runner group is only supported for organization scopes")
		return
	}
	labels, _, err := state.NormalizeWorkflowLabels(input.WorkflowLabels)
	name := strings.TrimSpace(input.Name)
	if err != nil || !validUserRunnerSpecName(name) || strings.TrimSpace(input.TemplateID) == "" || input.MaxConcurrency < 0 {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", "invalid runner type payload")
		return
	}
	if err := s.validateScopedProfileTemplate(r.Context(), prefScope, input.TemplateID); err != nil {
		s.writeScopedTemplateValidationError(w, err)
		return
	}
	profile := state.ScopedRunnerProfile{ScopeType: scope.Type, ScopeID: scope.ID, Name: name, WorkflowLabels: labels, TemplateID: strings.TrimSpace(input.TemplateID), RunnerGroup: strings.TrimSpace(input.RunnerGroup), MaxConcurrency: input.MaxConcurrency, Enabled: input.Enabled}
	err = s.applyMutationWithAudit("github:"+session.Subject, "user_runner_spec.create", "scoped_runner_profile", fmt.Sprintf("%s:%d:%s", scope.Type, scope.ID, profile.Name), map[string]any{"template_id": profile.TemplateID, "workflow_labels": labels}, func(tx state.Store) error {
		_, err := tx.UpsertScopedProfileIfUnchanged(profile, nil)
		return err
	})
	if err != nil {
		if errors.Is(err, state.ErrRunnerProfileNameConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_name_conflict", "runner type name conflicts with an enabled platform type")
			return
		}
		if errors.Is(err, state.ErrRunnerProfileLabelsConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_labels_conflict", "a runner type with these labels already exists")
			return
		}
		if errors.Is(err, state.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_name_conflict", "a runner type with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleUserListRunnerSpecs(w, r)
}

func (s *Server) handleUserPatchRunnerSpec(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, prefScope, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	var input userRunnerSpecPatchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", "invalid runner type payload")
		return
	}
	expected, ok := parseRunnerSpecRevision(w, input.ExpectedUpdatedAt, true)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	current, err := s.store.GetScopedProfile(scope, name)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, "runner_spec_not_found", "runner type not found")
		return
	}
	templateChanged := input.TemplateID != nil && strings.TrimSpace(*input.TemplateID) != current.TemplateID
	labelsChanged := false
	if input.WorkflowLabels != nil {
		labels, _, labelErr := state.NormalizeWorkflowLabels(*input.WorkflowLabels)
		if labelErr != nil {
			writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", labelErr.Error())
			return
		}
		labelsChanged = !sameStringSlice(labels, current.WorkflowLabels)
		current.WorkflowLabels = labels
	}
	if labelsChanged || templateChanged {
		count, countErr := s.store.ActiveCountForProfileScope("scoped_custom", scope, name)
		if countErr != nil {
			writeError(w, http.StatusInternalServerError, countErr.Error())
			return
		}
		if count > 0 {
			writeErrorCode(w, http.StatusConflict, "runner_spec_in_use", "runner type cannot change while active requests use it")
			return
		}
	}
	if templateChanged {
		if err := s.validateScopedProfileTemplate(r.Context(), prefScope, *input.TemplateID); err != nil {
			s.writeScopedTemplateValidationError(w, err)
			return
		}
		current.TemplateID = strings.TrimSpace(*input.TemplateID)
	}
	if input.RunnerGroup != nil {
		if scope.Type == state.AccountScopeTypeAccount && strings.TrimSpace(*input.RunnerGroup) != "" {
			writeErrorCode(w, http.StatusBadRequest, "runner_group_not_supported", "runner group is only supported for organization scopes")
			return
		}
		current.RunnerGroup = strings.TrimSpace(*input.RunnerGroup)
	}
	if input.MaxConcurrency != nil && *input.MaxConcurrency < 0 {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec", "max concurrency must not be negative")
		return
	}
	if input.MaxConcurrency != nil {
		current.MaxConcurrency = *input.MaxConcurrency
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	err = s.applyMutationWithAudit("github:"+session.Subject, "user_runner_spec.update", "scoped_runner_profile", fmt.Sprintf("%s:%d:%s", scope.Type, scope.ID, name), map[string]any{"template_id_changed": templateChanged, "workflow_labels_changed": labelsChanged}, func(tx state.Store) error {
		_, err := tx.UpsertScopedProfileIfUnchanged(current, &expected)
		return err
	})
	if err != nil {
		if errors.Is(err, state.ErrRunnerProfileLabelsConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_labels_conflict", "a runner type with these labels already exists")
			return
		}
		if errors.Is(err, state.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_conflict", "Runner type changed while saving; refresh and try again")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleUserListRunnerSpecs(w, r)
}

func validUserRunnerSpecName(name string) bool {
	return name != "" && !strings.Contains(name, "/") && name != "." && name != ".."
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func effectiveRunnerConcurrencyLimit(global, scope int) int {
	if global <= 0 {
		if scope <= 0 {
			return 0
		}
		return scope
	}
	if scope <= 0 || global < scope {
		return global
	}
	return scope
}

func (s *Server) handleUserDeleteRunnerSpec(w http.ResponseWriter, r *http.Request) {
	session, account, ok := s.requireUserSession(w, r)
	if !ok {
		return
	}
	scope, _, ok := s.userRunnerScope(w, r, account.ID)
	if !ok {
		return
	}
	expected, ok := parseRunnerSpecRevision(w, r.URL.Query().Get("expected_updated_at"), true)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if count, err := s.store.ActiveCountForProfileScope("scoped_custom", scope, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if count > 0 {
		writeErrorCode(w, http.StatusConflict, "runner_spec_in_use", "runner type is in use by active requests")
		return
	}
	err := s.applyMutationWithAudit("github:"+session.Subject, "user_runner_spec.delete", "scoped_runner_profile", fmt.Sprintf("%s:%d:%s", scope.Type, scope.ID, name), nil, func(tx state.Store) error {
		return tx.DeleteScopedProfileIfUnchanged(scope, name, &expected)
	})
	if err != nil {
		if errors.Is(err, state.ErrConflict) {
			writeErrorCode(w, http.StatusConflict, "runner_spec_conflict", "Runner type changed while deleting; refresh and try again")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleUserListRunnerSpecs(w, r)
}

func parseRunnerSpecRevision(w http.ResponseWriter, value string, required bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec_revision", "expected_updated_at is required")
			return time.Time{}, false
		}
		return time.Time{}, true
	}
	revision, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_runner_spec_revision", "expected_updated_at must be RFC3339")
		return time.Time{}, false
	}
	return revision, true
}

func (s *Server) validateScopedProfileTemplate(ctx context.Context, scope accountPreferenceScope, templateID string) error {
	_, snapshot, err := s.sandboxServiceForScope(scope)
	if err != nil {
		return errSandboxServiceNotConfigured
	}
	svc, err := s.sandboxServiceForConfig(snapshot)
	if err != nil {
		return err
	}
	validateCtx, cancel := context.WithTimeout(ctx, profileTemplateValidationTimeout)
	defer cancel()
	err = svc.ValidateTemplate(validateCtx, strings.TrimSpace(templateID))
	if err == nil {
		return validateCtx.Err()
	}
	return err
}

func (s *Server) writeScopedTemplateValidationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSandboxServiceNotConfigured):
		writeErrorCode(w, http.StatusConflict, "sandbox_service_not_configured", "configure Sandbox credentials for this scope before creating a custom runner type")
	case errors.Is(err, sandboxrunner.ErrTemplateNotFound):
		writeErrorCode(w, http.StatusBadRequest, "template_not_found", "template was not found in the scope Sandbox service")
	case errors.Is(err, sandboxrunner.ErrTemplateNotReady):
		writeErrorCode(w, http.StatusBadRequest, "template_not_ready", "template is not ready")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeErrorCode(w, http.StatusGatewayTimeout, "template_validation_timeout", "template validation timed out")
	default:
		writeErrorCode(w, http.StatusBadGateway, "template_validation_unavailable", "could not validate template with the scope Sandbox service")
	}
}
