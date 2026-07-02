package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/state"
)

func (s *Server) handleCreateRunner(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	var input manualCreateRequest
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			s.logger.Warn("manual runner payload rejected", "error", err)
			writeError(w, http.StatusBadRequest, "invalid runner payload")
			return
		}
	}
	id := input.ID
	if id == "" {
		id = newID()
	}
	labels := input.Labels
	requestedLabels := append([]string(nil), labels...)
	repositoryFullName := strings.TrimSpace(input.RepositoryFullName)
	if repositoryFullName == "" || strings.Contains(repositoryFullName, "*") {
		s.logger.Info("manual runner repository missing", "id", id, "repository", repositoryFullName)
		writeError(w, http.StatusBadRequest, "repository_full_name is required for manual runner creation")
		return
	}
	if !s.cfg.RepositoryAllowed(repositoryFullName) {
		s.logger.Info("manual runner repository rejected by allowlist", "id", id, "repository", repositoryFullName)
		writeError(w, http.StatusForbidden, "repository is not allowed")
		return
	}
	profileName := strings.TrimSpace(input.ProfileName)
	runnerGroup := ""
	if profileName == "" {
		if len(labels) == 0 {
			s.logger.Info("manual runner labels missing", "id", id, "repository", repositoryFullName)
			writeError(w, http.StatusBadRequest, "labels are required when runner_spec_name is not provided")
			return
		}
		match, err := s.store.MatchProfile(repositoryFullName, labels)
		if err != nil {
			s.logger.Error("match manual runner profile", "id", id, "repository", repositoryFullName, "labels", labels, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if match.Profile == nil {
			s.logger.Info("manual runner admission rejected", "id", id, "repository", repositoryFullName, "labels", labels, "reason", match.Reason)
			writeError(w, http.StatusBadRequest, "no matching profile")
			return
		}
		profileName = match.Profile.Name
		runnerGroup = match.Profile.RunnerGroup
		labels = append([]string(nil), match.Profile.Labels...)
	} else {
		profile, err := s.store.GetProfile(profileName)
		if err != nil {
			s.logger.Info("manual runner profile not found", "id", id, "repository", repositoryFullName, "profile", profileName)
			writeError(w, http.StatusBadRequest, "profile not found")
			return
		}
		if err := s.ensureRepositoryAllowsProfile(repositoryFullName, profile, labels); err != nil {
			s.logger.Info("manual runner repository policy rejected", "id", id, "repository", repositoryFullName, "profile", profileName, "labels", labels, "error", err)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		runnerGroup = profile.RunnerGroup
		if len(labels) == 0 {
			labels = append([]string(nil), profile.Labels...)
		}
		if len(requestedLabels) == 0 {
			requestedLabels = append([]string(nil), labels...)
		}
	}
	req := state.RunnerRequest{
		ID:                 id,
		Source:             "manual_api",
		RepositoryFullName: repositoryFullName,
		Labels:             labels,
		RequestedLabels:    requestedLabels,
		ProfileName:        profileName,
		RunnerGroup:        runnerGroup,
		RunnerName:         "e2b-" + id,
	}
	s.logger.Info("manual runner create requested", "id", id, "repository", repositoryFullName, "profile", profileName, "labels", labels)
	s.createAndStart(w, r, req, nil)
}

func (s *Server) handleListRunners(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	limit, offset, err := parsePagination(r, defaultRunnerRequestListLimit, maxRunnerRequestListLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	states, total, err := s.store.ListStatesPage(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writePaginationHeaders(w, r, total, limit, offset)
	writeJSON(w, http.StatusOK, states)
}

func (s *Server) handleGetRunner(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	st, err := s.store.ReadState(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "runner not found")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleRetryRunner(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	st, err := s.store.RetryRequest(r.PathValue("id"), time.Now().UTC())
	if err != nil {
		if errors.Is(err, state.ErrRetryNotAllowed) {
			s.logger.Info("runner retry rejected", "id", r.PathValue("id"), "error", err)
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.logger.Error("retry runner request", "id", r.PathValue("id"), "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.store.AppendLog(st.ID, "control.log", []byte("runner request manually requeued\n"))
	s.logger.Info("runner request manually requeued", "id", st.ID, "repository", st.RepositoryFullName, "profile", st.ProfileName)
	s.recordAudit("admin_api", "runner.retry", "runner_request", st.ID, map[string]any{
		"status":           st.Status,
		"repository":       st.RepositoryFullName,
		"runner_spec_name": st.ProfileName,
		"requested_labels": st.RequestedLabels,
	})
	s.refreshMetrics()
	writeJSON(w, http.StatusAccepted, st)
}

func (s *Server) handleGetRunnerLog(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	name := r.PathValue("name")
	switch name {
	case "control.log", "stdout.log", "stderr.log":
	default:
		writeError(w, http.StatusBadRequest, "unsupported log name")
		return
	}
	data, err := s.store.ReadLog(r.PathValue("id"), name, 256<<10)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			return
		}
		writeError(w, http.StatusNotFound, "log not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handleDeleteRunner(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	id := r.PathValue("id")
	st, _, err := s.stopRunner(context.Background(), id, github.WorkflowJob{})
	if err != nil {
		s.logger.Error("delete runner request failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("delete runner request handled", "id", id, "status", st.Status, "sandbox_id", st.SandboxID)
	s.recordAudit("admin_api", "runner.stop", "runner_request", id, map[string]any{
		"status": st.Status,
	})
	writeJSON(w, http.StatusAccepted, st)
}

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	events, err := s.store.ListAuditEvents(100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	s.refreshMetrics()
	profiles, err := s.store.ListProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	var input upsertProfileRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile payload")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	profile, err := s.store.UpsertProfile(state.RunnerProfile{
		Name:             input.Name,
		Labels:           input.Labels,
		TemplateID:       input.TemplateID,
		RunnerGroup:      input.RunnerGroup,
		MaxConcurrency:   input.MaxConcurrency,
		MinIdle:          intValue(input.MinIdle),
		Priority:         intValue(input.Priority),
		Enabled:          enabled,
		DefaultAvailable: input.DefaultAvailable != nil && *input.DefaultAvailable,
	})
	if err != nil {
		s.logger.Info("profile create rejected", "name", input.Name, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("profile created", "name", profile.Name, "labels", profile.Labels, "template_id", profile.TemplateID, "max_concurrency", profile.MaxConcurrency, "enabled", profile.Enabled)
	s.recordAudit("admin_api", "profile.create", "runner_profile", profile.Name, profile)
	s.refreshMetrics()
	writeJSON(w, http.StatusCreated, profile)
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	profile, err := s.store.GetProfile(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	current, err := s.store.GetProfile(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	var input upsertProfileRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile payload")
		return
	}
	if len(input.Labels) > 0 {
		current.Labels = input.Labels
	}
	if input.TemplateID != "" {
		current.TemplateID = input.TemplateID
	}
	if input.RunnerGroup != "" {
		current.RunnerGroup = input.RunnerGroup
	}
	if input.MaxConcurrency > 0 {
		current.MaxConcurrency = input.MaxConcurrency
	}
	if input.MinIdle != nil {
		current.MinIdle = *input.MinIdle
	}
	if input.Priority != nil {
		current.Priority = *input.Priority
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	if input.DefaultAvailable != nil {
		current.DefaultAvailable = *input.DefaultAvailable
	}
	profile, err := s.store.UpsertProfile(current)
	if err != nil {
		s.logger.Info("profile update rejected", "name", current.Name, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("profile updated", "name", profile.Name, "labels", profile.Labels, "template_id", profile.TemplateID, "max_concurrency", profile.MaxConcurrency, "enabled", profile.Enabled)
	s.recordAudit("admin_api", "profile.update", "runner_profile", profile.Name, profile)
	s.refreshMetrics()
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	if err := s.store.DeleteProfile(r.PathValue("name")); err != nil {
		s.logger.Error("delete profile", "name", r.PathValue("name"), "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("profile deleted", "name", r.PathValue("name"))
	s.recordAudit("admin_api", "profile.delete", "runner_profile", r.PathValue("name"), map[string]any{"status": "deleted"})
	s.refreshMetrics()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListRunnerGroups(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	groups, err := s.store.ListRunnerGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) handleCreateRunnerGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	var input upsertRunnerGroupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid runner group payload")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	group, err := s.store.UpsertRunnerGroup(state.RunnerGroup{
		Name:        input.Name,
		Description: input.Description,
		SpecNames:   input.SpecNames,
		Enabled:     enabled,
	})
	if err != nil {
		s.logger.Info("runner group create rejected", "name", input.Name, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("runner group created", "name", group.Name, "specs", group.SpecNames, "enabled", group.Enabled)
	s.recordAudit("admin_api", "runner_group.create", "runner_group", group.Name, group)
	writeJSON(w, http.StatusCreated, group)
}

func (s *Server) handleGetRunnerGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	group, err := s.store.GetRunnerGroup(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "runner group not found")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handlePatchRunnerGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	current, err := s.store.GetRunnerGroup(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "runner group not found")
		return
	}
	var input upsertRunnerGroupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid runner group payload")
		return
	}
	if input.Description != "" {
		current.Description = input.Description
	}
	if input.SpecNames != nil {
		current.SpecNames = input.SpecNames
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	group, err := s.store.UpsertRunnerGroup(current)
	if err != nil {
		s.logger.Info("runner group update rejected", "name", current.Name, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("runner group updated", "name", group.Name, "specs", group.SpecNames, "enabled", group.Enabled)
	s.recordAudit("admin_api", "runner_group.update", "runner_group", group.Name, group)
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleDeleteRunnerGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	name := r.PathValue("name")
	if err := s.store.DeleteRunnerGroup(name); err != nil {
		s.logger.Error("delete runner group", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("runner group deleted", "name", name)
	s.recordAudit("admin_api", "runner_group.delete", "runner_group", name, map[string]any{"status": "deleted"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListRepositoryPolicies(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	s.refreshMetrics()
	policies, err := s.store.ListRepositoryPolicies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

func (s *Server) handleCreateRepositoryPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	var input upsertRepositoryPolicyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid repository policy payload")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	policy, err := s.store.UpsertRepositoryPolicy(state.RepositoryPolicy{
		RepositoryFullName: input.RepositoryFullName,
		ProfileName:        input.ProfileName,
		RunnerGroupName:    input.RunnerGroupName,
		Enabled:            enabled,
	})
	if err != nil {
		s.logger.Info("repository policy create rejected", "repository", input.RepositoryFullName, "profile", input.ProfileName, "runner_group", input.RunnerGroupName, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("repository policy created", "id", policy.ID, "repository", policy.RepositoryFullName, "profile", policy.ProfileName, "runner_group", policy.RunnerGroupName, "enabled", policy.Enabled)
	s.recordAudit("admin_api", "repository_policy.create", "repository_policy", strconv.FormatInt(policy.ID, 10), policy)
	s.refreshMetrics()
	writeJSON(w, http.StatusCreated, policy)
}

func (s *Server) handlePatchRepositoryPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid policy id")
		return
	}
	existing, err := s.store.GetRepositoryPolicy(id)
	if errors.Is(err, state.ErrNotFound) {
		writeError(w, http.StatusNotFound, "repository policy not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var input upsertRepositoryPolicyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid repository policy payload")
		return
	}
	if input.RepositoryFullName != "" {
		existing.RepositoryFullName = input.RepositoryFullName
	}
	if input.ProfileName != "" {
		existing.ProfileName = input.ProfileName
		existing.RunnerGroupName = ""
	}
	if input.RunnerGroupName != "" {
		existing.RunnerGroupName = input.RunnerGroupName
		existing.ProfileName = ""
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}
	policy, err := s.store.UpsertRepositoryPolicy(existing)
	if err != nil {
		s.logger.Info("repository policy update rejected", "id", id, "repository", existing.RepositoryFullName, "profile", existing.ProfileName, "runner_group", existing.RunnerGroupName, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("repository policy updated", "id", policy.ID, "repository", policy.RepositoryFullName, "profile", policy.ProfileName, "runner_group", policy.RunnerGroupName, "enabled", policy.Enabled)
	s.recordAudit("admin_api", "repository_policy.update", "repository_policy", strconv.FormatInt(policy.ID, 10), policy)
	s.refreshMetrics()
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) handleDeleteRepositoryPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid policy id")
		return
	}
	if err := s.store.DeleteRepositoryPolicy(id); err != nil {
		s.logger.Error("delete repository policy", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("repository policy deleted", "id", id)
	s.recordAudit("admin_api", "repository_policy.delete", "repository_policy", strconv.FormatInt(id, 10), map[string]any{"status": "deleted"})
	s.refreshMetrics()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleMatchProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	var input profileMatchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid match payload")
		return
	}
	match, err := s.store.MatchProfile(input.RepositoryFullName, input.Labels)
	if err != nil {
		s.logger.Error("match profile request failed", "repository", input.RepositoryFullName, "labels", input.Labels, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if match.Profile == nil {
		s.logger.Info("match profile result", "repository", input.RepositoryFullName, "labels", input.Labels, "matched", false, "reason", match.Reason)
	} else {
		s.logger.Info("match profile result", "repository", input.RepositoryFullName, "labels", input.Labels, "matched", true, "profile", match.Profile.Name)
	}
	writeJSON(w, http.StatusOK, match)
}
