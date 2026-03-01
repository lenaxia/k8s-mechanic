package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
	sinkhub "github.com/lenaxia/k8s-mechanic/internal/sink/github"
)

// Compile-time interface check.
var _ domain.PRMergeChecker = (*sinkhub.GitHubPRMergeChecker)(nil)

func newMergeChecker(t *testing.T, token string, mux *routeHandler) (*sinkhub.GitHubPRMergeChecker, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	checker := &sinkhub.GitHubPRMergeChecker{
		TokenProvider: &staticTokenProvider{token: token},
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
	}
	return checker, srv
}

func prRef(repo string, number int) v1alpha1.SinkRef {
	return v1alpha1.SinkRef{
		Type:   "pr",
		URL:    "https://github.com/" + repo + "/pull/42",
		Number: number,
		Repo:   repo,
	}
}

func TestGitHubPRMergeChecker_Merged(t *testing.T) {
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/pulls/42/merge": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent) // 204 = merged
		},
	}}
	checker, _ := newMergeChecker(t, "tok", mux)
	merged, err := checker.IsMerged(context.Background(), prRef("org/repo", 42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !merged {
		t.Error("expected merged=true, got false")
	}
}

func TestGitHubPRMergeChecker_NotMerged(t *testing.T) {
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/pulls/42/merge": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound) // 404 = not merged
		},
	}}
	checker, _ := newMergeChecker(t, "tok", mux)
	merged, err := checker.IsMerged(context.Background(), prRef("org/repo", 42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged {
		t.Error("expected merged=false, got true")
	}
}

func TestGitHubPRMergeChecker_TransientError(t *testing.T) {
	mux := &routeHandler{handlers: map[string]http.HandlerFunc{
		"GET /repos/org/repo/pulls/42/merge": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError) // unexpected → error
		},
	}}
	checker, _ := newMergeChecker(t, "tok", mux)
	merged, err := checker.IsMerged(context.Background(), prRef("org/repo", 42))
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if merged {
		t.Error("expected merged=false on error")
	}
}

func TestGitHubPRMergeChecker_EmptyURL(t *testing.T) {
	checker := &sinkhub.GitHubPRMergeChecker{}
	merged, err := checker.IsMerged(context.Background(), v1alpha1.SinkRef{})
	if err != nil {
		t.Fatalf("unexpected error for empty ref: %v", err)
	}
	if merged {
		t.Error("expected merged=false for empty ref")
	}
}

func TestGitHubPRMergeChecker_NonPRType(t *testing.T) {
	checker := &sinkhub.GitHubPRMergeChecker{}
	ref := v1alpha1.SinkRef{Type: "issue", URL: "https://github.com/org/repo/issues/1", Number: 1, Repo: "org/repo"}
	merged, err := checker.IsMerged(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error for issue ref: %v", err)
	}
	if merged {
		t.Error("expected merged=false for non-pr type")
	}
}

func TestGitHubPRMergeChecker_TokenError(t *testing.T) {
	checker := &sinkhub.GitHubPRMergeChecker{
		TokenProvider: &staticTokenProvider{err: errFakeToken},
		BaseURL:       "http://localhost:1", // unreachable — should fail at token
	}
	merged, err := checker.IsMerged(context.Background(), prRef("org/repo", 42))
	if err == nil {
		t.Fatal("expected error when token provider fails")
	}
	if merged {
		t.Error("expected merged=false on token error")
	}
}
