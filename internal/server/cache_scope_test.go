package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qiniu/ci-runner/internal/github"
	"github.com/qiniu/ci-runner/internal/state"
)

func TestCacheScopesForWorkflowMatchesGitHubScopes(t *testing.T) {
	tests := []struct {
		name           string
		run            string
		pull           string
		wantWriteScope string
		wantReadScopes []string
		wantDecision   string
	}{
		{
			name:           "internal pull request reads PR base and default scopes",
			run:            `{"id":101,"event":"pull_request","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"},"pull_requests":[{"number":7}]}`,
			pull:           `{"number":7,"head":{"ref":"feature","repo":{"full_name":"owner/repo"}},"base":{"ref":"release","repo":{"full_name":"owner/repo"}}}`,
			wantWriteScope: scopeForPR(7),
			wantReadScopes: []string{scopeForPR(7), scopeForBranch("release"), scopeForBranch("master")},
			wantDecision:   "internal_pull_request",
		},
		{
			name:           "fork pull request writes only its PR scope",
			run:            `{"id":101,"event":"pull_request","head_repository":{"full_name":"fork/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"},"pull_requests":[{"number":7}]}`,
			pull:           `{"number":7,"head":{"ref":"feature","repo":{"full_name":"fork/repo"}},"base":{"ref":"master","repo":{"full_name":"owner/repo"}}}`,
			wantWriteScope: scopeForPR(7),
			wantReadScopes: []string{scopeForPR(7), scopeForBranch("master")},
			wantDecision:   "fork_pull_request",
		},
		{
			name:           "trusted branch reads itself then default",
			run:            `{"id":101,"event":"push","head_branch":"feature","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"}}`,
			wantWriteScope: scopeForBranch("feature"),
			wantReadScopes: []string{scopeForBranch("feature"), scopeForBranch("master")},
			wantDecision:   "branch",
		},
		{
			name:           "pull request target is default scope read only",
			run:            `{"id":101,"event":"pull_request_target","head_branch":"feature","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"}}`,
			wantReadScopes: []string{scopeForBranch("master")},
			wantDecision:   "read_only_pull_request_target",
		},
		{
			name:           "untrusted event is default scope read only",
			run:            `{"id":101,"event":"workflow_call","head_branch":"feature","head_repository":{"full_name":"owner/repo"},"repository":{"full_name":"owner/repo","default_branch":"master"}}`,
			wantReadScopes: []string{scopeForBranch("master")},
			wantDecision:   "read_only_untrusted_event",
		},
		{
			name:           "untrusted PR metadata is default scope read only",
			run:            `{"id":101,"event":"pull_request","repository":{"full_name":"owner/repo","default_branch":"master"},"pull_requests":[{"number":7}]}`,
			pull:           `{"number":7,"head":{"ref":"feature","repo":{"full_name":"fork/repo"}},"base":{"ref":"main","repo":{"full_name":"other/repo"}}}`,
			wantReadScopes: []string{scopeForBranch("master")},
			wantDecision:   "read_only_untrusted_pull_request_metadata",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repos/owner/repo/actions/runs/101":
					_, _ = w.Write([]byte(test.run))
				case "/repos/owner/repo/pulls/7":
					if test.pull == "" {
						t.Fatalf("unexpected pull request lookup")
					}
					_, _ = w.Write([]byte(test.pull))
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer api.Close()

			srv := &Server{gh: github.NewClient(api.URL, api.Client())}
			got := srv.cacheScopesForWorkflow(t.Context(), "owner/repo", state.RunnerState{WorkflowRunID: 101}, scopeForBranch("main"))
			if got.WriteScope != test.wantWriteScope || got.Decision != test.wantDecision {
				t.Fatalf("decision = %#v", got)
			}
			if len(got.ReadScopes) != len(test.wantReadScopes) {
				t.Fatalf("read scopes = %#v, want %#v", got.ReadScopes, test.wantReadScopes)
			}
			for i := range test.wantReadScopes {
				if got.ReadScopes[i] != test.wantReadScopes[i] {
					t.Fatalf("read scopes = %#v, want %#v", got.ReadScopes, test.wantReadScopes)
				}
			}
		})
	}
}

func TestCacheScopesForWorkflowFailsClosedWithoutWorkflowRun(t *testing.T) {
	defaultScope := scopeForBranch("main")
	got := (&Server{}).cacheScopesForWorkflow(t.Context(), "owner/repo", state.RunnerState{}, defaultScope)
	if got.WriteScope != "" || got.Decision != "read_only_missing_workflow_run" || len(got.ReadScopes) != 1 || got.ReadScopes[0] != defaultScope {
		t.Fatalf("scope decision = %#v", got)
	}
}

func TestCacheScopePrefixesJSON(t *testing.T) {
	got, err := cacheScopePrefixesJSON("gh-actions-cache/owner/repo/", []string{"scopes/pr-7", "scopes/branch-main", "scopes/pr-7"})
	if err != nil {
		t.Fatal(err)
	}
	want := `["gh-actions-cache/owner/repo/scopes/pr-7","gh-actions-cache/owner/repo/scopes/branch-main"]`
	if got != want {
		t.Fatalf("prefix JSON = %s, want %s", got, want)
	}
}
