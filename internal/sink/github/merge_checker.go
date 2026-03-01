package github

import (
	"context"
	"fmt"
	"io"
	"net/http"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
	igithub "github.com/lenaxia/k8s-mechanic/internal/github"
)

// Compile-time interface check.
var _ domain.PRMergeChecker = (*GitHubPRMergeChecker)(nil)

// GitHubPRMergeChecker checks whether a GitHub PR has been merged using the
// GitHub REST API.
//
// It calls GET /repos/{owner}/{repo}/pulls/{number}/merge:
//   - 204 No Content  → merged
//   - 404 Not Found   → not merged (PR open or closed without merge)
//   - other           → transient error, returned to caller
type GitHubPRMergeChecker struct {
	TokenProvider igithub.TokenProvider
	// BaseURL allows overriding the GitHub API endpoint in tests.
	// Defaults to "https://api.github.com" when empty.
	BaseURL    string
	HTTPClient *http.Client
}

// IsMerged returns true if the PR referenced by ref has been merged.
// Returns (false, nil) if ref.URL is empty or ref.Type is not "pr".
// Returns (false, nil) on 404 (PR exists but not merged).
// Returns (false, err) on network or auth errors.
func (c *GitHubPRMergeChecker) IsMerged(ctx context.Context, ref v1alpha1.SinkRef) (bool, error) {
	if ref.URL == "" || ref.Type != "pr" {
		return false, nil
	}
	if ref.Number <= 0 {
		return false, fmt.Errorf("IsMerged: SinkRef.Number must be > 0, got %d", ref.Number)
	}
	if ref.Repo == "" {
		return false, fmt.Errorf("IsMerged: SinkRef.Repo must not be empty")
	}

	token, err := c.TokenProvider.Token(ctx)
	if err != nil {
		return false, fmt.Errorf("IsMerged: getting GitHub token: %w", err)
	}

	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}

	// GET /repos/{owner}/{repo}/pulls/{number}/merge
	// 204 = merged, 404 = not merged, anything else = error.
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/merge", base, ref.Repo, ref.Number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("IsMerged: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("IsMerged: executing request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusNoContent: // 204
		return true, nil
	case http.StatusNotFound: // 404
		return false, nil
	default:
		return false, fmt.Errorf("IsMerged: unexpected status %d for %s", resp.StatusCode, url)
	}
}
