package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/labelutil"
	"gorm.io/gorm"
)

func recordToRequest(record runnerRequestRecord) (RunnerRequest, error) {
	var labels []string
	if err := json.Unmarshal([]byte(record.LabelsJSON), &labels); err != nil {
		return RunnerRequest{}, err
	}
	var requestedLabels []string
	if record.RequestedLabelsJSON != "" {
		if err := json.Unmarshal([]byte(record.RequestedLabelsJSON), &requestedLabels); err != nil {
			return RunnerRequest{}, err
		}
	}
	return RunnerRequest{
		ID:                     record.ID,
		Source:                 record.Source,
		JobID:                  pointerToInt64(record.WorkflowJobID),
		GitHubInstallationID:   record.GitHubInstallationID,
		RepositoryFullName:     record.RepositoryFullName,
		RequestedLabels:        requestedLabels,
		Labels:                 labels,
		ProfileName:            record.ProfileName,
		RunnerGroup:            record.RunnerGroup,
		RunnerName:             record.RunnerName,
		SandboxAPIURL:          record.SandboxAPIURL,
		SandboxAPIKeyEncrypted: record.SandboxAPIKeyEncrypted,
		CreatedAt:              record.QueuedAt,
	}, nil
}

func recordToState(record runnerRequestRecord) RunnerState {
	requestedLabels, _ := labelsFromJSON(record.RequestedLabelsJSON)
	githubLinks := githubLinksFromRecord(record)
	return RunnerState{
		ID:                     record.ID,
		Status:                 record.Status,
		GitHubInstallationID:   record.GitHubInstallationID,
		RepositoryFullName:     record.RepositoryFullName,
		RequestedLabels:        requestedLabels,
		ProfileName:            record.ProfileName,
		RunnerGroup:            record.RunnerGroup,
		RunnerName:             record.RunnerName,
		SandboxID:              record.SandboxID,
		SandboxAPIURL:          record.SandboxAPIURL,
		SandboxAPIKeyEncrypted: record.SandboxAPIKeyEncrypted,
		ProcessPID:             record.ProcessPID,
		WorkflowJobID:          pointerToInt64(record.WorkflowJobID),
		WorkflowRunID:          githubLinks.workflowRunID,
		WorkflowName:           githubLinks.workflowName,
		WorkflowRunAttempt:     githubLinks.workflowRunAttempt,
		HeadBranch:             githubLinks.headBranch,
		HeadSHA:                githubLinks.headSHA,
		GitHubJobURL:           githubLinks.jobURL,
		PullRequestNumber:      githubLinks.pullRequestNumber,
		AssignedJobID:          record.AssignedJobID,
		AssignedJobName:        record.AssignedJobName,
		Error:                  record.Error,
		FailureStage:           record.FailureStage,
		FailureReason:          record.FailureReason,
		LastErrorCode:          record.LastErrorCode,
		LastErrorMessage:       record.LastErrorMessage,
		LastErrorRetryable:     record.LastErrorRetryable,
		RetryCount:             record.RetryCount,
		UpdatedAt:              record.UpdatedAt,
		CreatedAt:              record.QueuedAt,
		LastAttemptAt:          pointerToTime(record.LastAttemptAt),
		NextRetryAt:            pointerToTime(record.NextRetryAt),
		CreatingAt:             pointerToTime(record.CreatingAt),
		RunningAt:              pointerToTime(record.RunningAt),
		StoppingAt:             pointerToTime(record.StoppingAt),
		CompletedAt:            pointerToTime(record.CompletedAt),
		FailedAt:               pointerToTime(record.FailedAt),
		LeaseOwner:             record.LeaseOwner,
		LeaseExpiresAt:         pointerToTime(record.LeaseExpiresAt),
		Version:                record.Version,
	}
}

func githubLinksFromRecord(record runnerRequestRecord) githubPayloadLinks {
	links := githubPayloadLinks{
		workflowRunID:      record.WorkflowRunID,
		workflowName:       record.WorkflowName,
		workflowRunAttempt: record.WorkflowRunAttempt,
		headBranch:         record.HeadBranch,
		headSHA:            record.HeadSHA,
		jobURL:             record.GitHubJobURL,
		pullRequestNumber:  record.PullRequestNumber,
	}
	if !record.GitHubContextBackfilled {
		links = mergeGitHubPayloadLinks(links, githubLinksFromPayload(record))
	}
	if links.jobURL == "" && record.RepositoryFullName != "" && links.workflowRunID > 0 {
		jobID := record.AssignedJobID
		if jobID == 0 {
			jobID = pointerToInt64(record.WorkflowJobID)
		}
		if jobID > 0 {
			links.jobURL = fmt.Sprintf("https://github.com/%s/actions/runs/%d/job/%d", record.RepositoryFullName, links.workflowRunID, jobID)
		}
	}
	links.jobURL = appendPullRequestQuery(links.jobURL, links.pullRequestNumber)
	return links
}

func mergeGitHubPayloadLinks(links githubPayloadLinks, fallback githubPayloadLinks) githubPayloadLinks {
	if links.workflowRunID == 0 {
		links.workflowRunID = fallback.workflowRunID
	}
	if links.workflowName == "" {
		links.workflowName = fallback.workflowName
	}
	if links.workflowRunAttempt == 0 {
		links.workflowRunAttempt = fallback.workflowRunAttempt
	}
	if links.headBranch == "" {
		links.headBranch = fallback.headBranch
	}
	if links.headSHA == "" {
		links.headSHA = fallback.headSHA
	}
	if links.jobURL == "" {
		links.jobURL = fallback.jobURL
	}
	if links.pullRequestNumber == 0 {
		links.pullRequestNumber = fallback.pullRequestNumber
	}
	return links
}

type githubPayloadLinks struct {
	workflowRunID      int64
	workflowName       string
	workflowRunAttempt int64
	headBranch         string
	headSHA            string
	jobURL             string
	pullRequestNumber  int64
}

type githubPayloadPullRequest struct {
	Number int64 `json:"number"`
}

func githubLinksFromPayload(record runnerRequestRecord) githubPayloadLinks {
	var payload struct {
		WorkflowJob struct {
			RunID        int64                      `json:"run_id"`
			WorkflowName string                     `json:"workflow_name"`
			RunAttempt   int64                      `json:"run_attempt"`
			HeadBranch   string                     `json:"head_branch"`
			HeadSHA      string                     `json:"head_sha"`
			HTMLURL      string                     `json:"html_url"`
			PullRequests []githubPayloadPullRequest `json:"pull_requests"`
			WorkflowRun  struct {
				ID int64 `json:"id"`
			} `json:"workflow_run"`
		} `json:"workflow_job"`
		WorkflowRun struct {
			ID           int64                      `json:"id"`
			Name         string                     `json:"name"`
			RunAttempt   int64                      `json:"run_attempt"`
			HeadBranch   string                     `json:"head_branch"`
			HeadSHA      string                     `json:"head_sha"`
			HTMLURL      string                     `json:"html_url"`
			PullRequests []githubPayloadPullRequest `json:"pull_requests"`
		} `json:"workflow_run"`
		PullRequest struct {
			Number int64 `json:"number"`
		} `json:"pull_request"`
	}
	if record.GitHubPayloadJSON != "" {
		_ = json.Unmarshal([]byte(record.GitHubPayloadJSON), &payload)
	}

	runID := payload.WorkflowJob.RunID
	if runID == 0 {
		runID = payload.WorkflowJob.WorkflowRun.ID
	}
	if runID == 0 {
		runID = payload.WorkflowRun.ID
	}
	workflowName := firstNonEmpty(payload.WorkflowJob.WorkflowName, payload.WorkflowRun.Name)
	runAttempt := payload.WorkflowJob.RunAttempt
	if runAttempt == 0 {
		runAttempt = payload.WorkflowRun.RunAttempt
	}
	headBranch := firstNonEmpty(payload.WorkflowJob.HeadBranch, payload.WorkflowRun.HeadBranch)
	headSHA := firstNonEmpty(payload.WorkflowJob.HeadSHA, payload.WorkflowRun.HeadSHA)

	prNumber := firstPullRequestNumber(payload.WorkflowJob.PullRequests)
	if prNumber == 0 {
		prNumber = firstPullRequestNumber(payload.WorkflowRun.PullRequests)
	}
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}

	jobURL := payload.WorkflowJob.HTMLURL
	jobID := record.AssignedJobID
	if jobID == 0 {
		jobID = pointerToInt64(record.WorkflowJobID)
	}
	if jobURL == "" && record.RepositoryFullName != "" && runID > 0 && jobID > 0 {
		jobURL = fmt.Sprintf("https://github.com/%s/actions/runs/%d/job/%d", record.RepositoryFullName, runID, jobID)
	}
	jobURL = appendPullRequestQuery(jobURL, prNumber)

	return githubPayloadLinks{
		workflowRunID:      runID,
		workflowName:       workflowName,
		workflowRunAttempt: runAttempt,
		headBranch:         headBranch,
		headSHA:            headSHA,
		jobURL:             jobURL,
		pullRequestNumber:  prNumber,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPullRequestNumber(pulls []githubPayloadPullRequest) int64 {
	if len(pulls) == 0 {
		return 0
	}
	return pulls[0].Number
}

func appendPullRequestQuery(rawURL string, prNumber int64) string {
	if rawURL == "" || prNumber == 0 || strings.Contains(rawURL, "pr=") {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%spr=%d", rawURL, separator, prNumber)
}

func recordToProfile(record runnerProfileRecord) (RunnerProfile, error) {
	var labels []string
	if err := json.Unmarshal([]byte(record.LabelsJSON), &labels); err != nil {
		return RunnerProfile{}, err
	}
	return RunnerProfile{
		Name:             record.Name,
		Labels:           labels,
		TemplateID:       record.TemplateID,
		RunnerGroup:      record.RunnerGroup,
		MaxConcurrency:   record.MaxConcurrency,
		MinIdle:          record.MinIdle,
		Priority:         record.Priority,
		Enabled:          record.Enabled,
		DefaultAvailable: record.DefaultAvailable,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}, nil
}

func (s *DBStore) recordToRunnerGroup(db *gorm.DB, record runnerGroupRecord) (RunnerGroup, error) {
	var links []runnerGroupSpecRecord
	if err := db.Where("group_name = ?", record.Name).Order("spec_name ASC").Find(&links).Error; err != nil {
		return RunnerGroup{}, err
	}
	specNames := make([]string, 0, len(links))
	for _, link := range links {
		specNames = append(specNames, link.SpecName)
	}
	return RunnerGroup{
		Name:        record.Name,
		Description: record.Description,
		SpecNames:   specNames,
		Enabled:     record.Enabled,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}, nil
}

func recordToRepositoryPolicy(record repositoryPolicyRecord) RepositoryPolicy {
	//lint:ignore S1016 keep record/API mapping explicit so field changes are reviewed intentionally
	return RepositoryPolicy{
		ID:                 record.ID,
		RepositoryFullName: record.RepositoryFullName,
		ProfileName:        record.ProfileName,
		RunnerGroupName:    record.RunnerGroupName,
		Enabled:            record.Enabled,
		CreatedAt:          record.CreatedAt,
	}
}

func uniqueTrimmed(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func zeroTimeToPointer(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	tt := t.UTC()
	return &tt
}

func pointerToTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

func pointerToInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func labelsFromJSON(data string) ([]string, error) {
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(data), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func repositoryMatches(pattern, repository string) bool {
	pattern = strings.TrimSpace(pattern)
	repository = strings.TrimSpace(repository)
	if pattern == "" || repository == "" {
		return false
	}
	if pattern == repository {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(repository, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func labelsMatch(jobLabels, required []string) bool {
	return labelutil.Match(jobLabels, required)
}

func logEventType(name string) (string, error) {
	switch filepath.Base(name) {
	case "control.log":
		return "control_log", nil
	case "stdout.log":
		return "stdout_log", nil
	case "stderr.log":
		return "stderr_log", nil
	default:
		return "", fmt.Errorf("unsupported log name")
	}
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "\\", "-")
	id = strings.ReplaceAll(id, "..", "-")
	return id
}

func sanitizeRunnerName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}

func normalizeOAuthProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func normalizeOAuthSubject(subject string) string {
	return strings.TrimSpace(subject)
}

func normalizeOAuthLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func normalizePlatformRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "admin", "user":
		return role
	default:
		return ""
	}
}
