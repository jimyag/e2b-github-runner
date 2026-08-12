package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/state"
)

func TestCacheScopesForWorkflowUsesVerifiedWorkflowRun(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		wantOwnScope  string
		wantDecision  string
		wantReadCount int
	}{
		{
			name:          "fork pull request is read only",
			response:      `{"id":101,"event":"pull_request","head_branch":"main","head_repository":{"full_name":"fork/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"},"pull_requests":[{"number":7}]}`,
			wantOwnScope:  "",
			wantDecision:  "read_only_fork_pull_request",
			wantReadCount: 1,
		},
		{
			name:          "internal pull request writes only PR scope",
			response:      `{"id":101,"event":"pull_request","head_branch":"feature","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"},"pull_requests":[{"number":7}]}`,
			wantOwnScope:  scopeForPR(7),
			wantDecision:  "internal_pull_request",
			wantReadCount: 3,
		},
		{
			name:          "branch workflow writes only branch scope",
			response:      `{"id":101,"event":"push","head_branch":"main","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"}}`,
			wantOwnScope:  scopeForBranch("main"),
			wantDecision:  "branch",
			wantReadCount: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/owner/repo/actions/runs/101" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.response))
			}))
			defer api.Close()
			srv := &Server{gh: github.NewClient(api.URL, api.Client())}
			defaultScope := scopeForBranch("main")
			ownScope, readScopes, decision := srv.cacheScopesForWorkflow(t.Context(), "owner/repo", state.RunnerState{WorkflowRunID: 101}, defaultScope)
			if ownScope != test.wantOwnScope || decision != test.wantDecision || len(readScopes) != test.wantReadCount {
				t.Fatalf("scope result = own=%q read=%#v decision=%q", ownScope, readScopes, decision)
			}
		})
	}
}

func TestCacheScopesForWorkflowFailsClosedWithoutWorkflowRun(t *testing.T) {
	defaultScope := scopeForBranch("main")
	ownScope, readScopes, decision := (&Server{}).cacheScopesForWorkflow(t.Context(), "owner/repo", state.RunnerState{}, defaultScope)
	if ownScope != "" || decision != "read_only_missing_workflow_run" || len(readScopes) != 1 || readScopes[0] != defaultScope {
		t.Fatalf("scope result = own=%q read=%#v decision=%q", ownScope, readScopes, decision)
	}
}
