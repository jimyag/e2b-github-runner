package metrics

import (
	"expvar"
	"strconv"
	"time"

	"github.com/qiniu/ci-runner/internal/state"
)

var (
	profileCurrent       = expvar.NewMap("e2b_runner_profile_current")
	profileBusy          = expvar.NewMap("e2b_runner_profile_busy")
	profileIdle          = expvar.NewMap("e2b_runner_profile_idle")
	profileDesired       = expvar.NewMap("e2b_runner_profile_desired")
	profilePending       = expvar.NewMap("e2b_runner_profile_pending")
	profileStatus        = expvar.NewMap("e2b_runner_profile_status")
	requestsByStatus     = expvar.NewMap("e2b_runner_requests_by_profile_status")
	requestsByRepository = expvar.NewMap("e2b_runner_requests_by_repository_status")
	retryState           = expvar.NewMap("e2b_runner_retry_state")
	leaseState           = expvar.NewMap("e2b_runner_lease_state")
	createDuration       = expvar.NewMap("e2b_runner_create_duration_ms_total")
	createCount          = expvar.NewMap("e2b_runner_create_duration_count")
	stopDuration         = expvar.NewMap("e2b_runner_stop_duration_ms_total")
	stopCount            = expvar.NewMap("e2b_runner_stop_duration_count")
	operationsTotal      = expvar.NewMap("e2b_runner_operations_total")
	runnerRegistered     = expvar.NewMap("e2b_runner_registered_total")
	runnerCleanup        = expvar.NewMap("e2b_runner_cleanup_total")
	githubAPIOperations  = expvar.NewMap("e2b_runner_github_api_operations_total")
	githubAPIDuration    = expvar.NewMap("e2b_runner_github_api_duration_ms_total")
	httpRequestsTotal    = expvar.NewMap("e2b_runner_http_requests_total")
	httpRequestDuration  = expvar.NewMap("e2b_runner_http_request_duration_ms_total")
	workflowQueued       = expvar.NewMap("github_workflow_jobs_queued_total")
	workflowStarted      = expvar.NewMap("github_workflow_jobs_started_total")
	workflowComplete     = expvar.NewMap("github_workflow_jobs_completed_total")
	workflowConclusions  = expvar.NewMap("github_workflow_job_conclusions_total")
	workflowFailures     = expvar.NewMap("github_workflow_job_failures_total")
	workflowQueueTime    = expvar.NewMap("github_workflow_job_queue_duration_ms_total")
	workflowQueueCount   = expvar.NewMap("github_workflow_job_queue_duration_count")
	workflowRunTime      = expvar.NewMap("github_workflow_job_run_duration_ms_total")
	workflowRunCount     = expvar.NewMap("github_workflow_job_run_duration_count")
	runnerRequestsTotal  = expvar.NewMap("e2b_runner_requests_total")
	lastReconcileUnix    = expvar.NewInt("e2b_runner_last_reconcile_unix")
)

func Refresh(profiles []state.RunnerProfile, states []state.RunnerState) {
	current := map[string]int64{}
	busy := map[string]int64{}
	idle := map[string]int64{}
	pending := map[string]int64{}
	byStatus := map[string]int64{}
	byRepository := map[string]int64{}
	retries := map[string]int64{}
	leases := map[string]int64{}
	for _, st := range states {
		profile := st.ProfileName
		if profile == "" {
			profile = "unassigned"
		}
		repository := st.RepositoryFullName
		if repository == "" {
			repository = "unassigned"
		}
		switch st.Status {
		case state.StatusQueued, state.StatusCreating:
			pending[profile]++
		case state.StatusRunning, state.StatusStopping:
			current[profile]++
			if st.AssignedJobID != 0 || st.AssignedJobName != "" {
				busy[profile]++
			} else {
				idle[profile]++
			}
		}
		byStatus[metricJoin(profile, st.Status)]++
		byRepository[metricJoin(repository, st.Status)]++
		if st.RetryCount > 0 || !st.NextRetryAt.IsZero() {
			retries[metricJoin(profile, st.Status)]++
		}
		if st.LeaseOwner != "" && !st.LeaseExpiresAt.IsZero() {
			leases[st.LeaseOwner]++
		}
	}
	profileCurrent.Init()
	profileBusy.Init()
	profileIdle.Init()
	profileDesired.Init()
	profilePending.Init()
	profileStatus.Init()
	requestsByStatus.Init()
	requestsByRepository.Init()
	retryState.Init()
	leaseState.Init()
	for _, profile := range profiles {
		profileCurrent.Set(profile.Name, newInt(current[profile.Name]))
		profileBusy.Set(profile.Name, newInt(busy[profile.Name]))
		profileIdle.Set(profile.Name, newInt(idle[profile.Name]))
		profileDesired.Set(profile.Name, newInt(int64(profile.MaxConcurrency)))
		profilePending.Set(profile.Name, newInt(pending[profile.Name]))
		if profile.Enabled {
			profileStatus.Set(profile.Name, newInt(1))
		} else {
			profileStatus.Set(profile.Name, newInt(0))
		}
	}
	for key, value := range byStatus {
		requestsByStatus.Set(metricKey(key), newInt(value))
	}
	for key, value := range byRepository {
		requestsByRepository.Set(metricKey(key), newInt(value))
	}
	for key, value := range retries {
		retryState.Set(metricKey(key), newInt(value))
	}
	for key, value := range leases {
		leaseState.Set(metricKey(key), newInt(value))
	}
	lastReconcileUnix.Set(time.Now().UTC().Unix())
}

func RecordCreate(profile string, d time.Duration, result string) {
	createDuration.Add(profile, d.Milliseconds())
	createCount.Add(profile, 1)
	operationsTotal.Add(metricKey(profile, "create", result), 1)
}

func RecordStop(profile string, d time.Duration, result string) {
	stopDuration.Add(profile, d.Milliseconds())
	stopCount.Add(profile, 1)
	operationsTotal.Add(metricKey(profile, "stop", result), 1)
}

func RecordRunnerRegistered(profile, result string) {
	runnerRegistered.Add(metricKey(profile, result), 1)
}

func RecordRunnerCleanup(profile, result string) {
	runnerCleanup.Add(metricKey(profile, result), 1)
}

func RecordRunnerRequest(repository, profile, source, result string) {
	runnerRequestsTotal.Add(metricKey(repository, profile, source, result), 1)
}

func RecordGitHubAPI(operation, result string, d time.Duration) {
	githubAPIOperations.Add(metricKey(operation, result), 1)
	githubAPIDuration.Add(metricKey(operation, result), d.Milliseconds())
}

func RecordHTTPRequest(method, route string, status int, d time.Duration) {
	key := metricKey(method, route, strconv.Itoa(status))
	httpRequestsTotal.Add(key, 1)
	httpRequestDuration.Add(key, d.Milliseconds())
}

func RecordWorkflowQueued(repository, workflow, job, profile string) {
	workflowQueued.Add(metricKey(repository, workflow, job, profile), 1)
}

func RecordWorkflowStarted(repository, workflow, job, profile string) {
	workflowStarted.Add(metricKey(repository, workflow, job, profile), 1)
}

func RecordWorkflowCompleted(repository, workflow, job, profile, conclusion string) {
	workflowComplete.Add(metricKey(repository, workflow, job, profile, conclusion), 1)
	workflowConclusions.Add(metricKey(repository, workflow, job, profile, conclusion), 1)
}

func RecordWorkflowFailure(repository, workflow, job, profile, failedStep, exitCode string) {
	workflowFailures.Add(metricKey(repository, workflow, job, profile, failedStep, exitCode), 1)
}

func RecordWorkflowQueueDuration(repository, workflow, job, profile string, d time.Duration) {
	if d < 0 {
		return
	}
	key := metricKey(repository, workflow, job, profile)
	workflowQueueTime.Add(key, d.Milliseconds())
	workflowQueueCount.Add(key, 1)
}

func RecordWorkflowRunDuration(repository, workflow, job, profile, conclusion string, d time.Duration) {
	if d < 0 {
		return
	}
	key := metricKey(repository, workflow, job, profile, conclusion)
	workflowRunTime.Add(key, d.Milliseconds())
	workflowRunCount.Add(key, 1)
}

func metricKey(parts ...string) string {
	return metricJoin(parts...)
}

func metricJoin(parts ...string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += "|"
		}
		out += part
	}
	return out
}

func newInt(v int64) *expvar.Int {
	value := new(expvar.Int)
	value.Set(v)
	return value
}
