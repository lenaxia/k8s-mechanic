package domain

import (
	"context"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

// SinkCloser closes an open sink (PR or issue) when the underlying finding resolves.
// Implementations must be idempotent: closing an already-closed sink returns nil.
// Close returns nil immediately if rjob.Status.SinkRef.URL is empty.
type SinkCloser interface {
	Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error
}

// NoopSinkCloser is a SinkCloser that does nothing.
// Used when PR_AUTO_CLOSE=false or in tests that do not need real closure.
type NoopSinkCloser struct{}

func (NoopSinkCloser) Close(_ context.Context, _ *v1alpha1.RemediationJob, _ string) error {
	return nil
}

// PRMergeChecker checks whether the GitHub PR referenced by a SinkRef has been
// merged.  Implementations must be safe to call concurrently and must be
// idempotent (repeated calls for the same PR return the same result once merged).
//
// IsMerged returns (true, nil) when the PR is confirmed merged.
// IsMerged returns (false, nil) when the PR is open or closed but not merged.
// IsMerged returns (false, err) on transient errors (network, auth, rate-limit).
// IsMerged returns (false, nil) immediately when ref.URL is empty.
type PRMergeChecker interface {
	IsMerged(ctx context.Context, ref v1alpha1.SinkRef) (bool, error)
}

// NoopPRMergeChecker always reports not-merged.
// Used in tests or when no GitHub credentials are configured.
type NoopPRMergeChecker struct{}

func (NoopPRMergeChecker) IsMerged(_ context.Context, _ v1alpha1.SinkRef) (bool, error) {
	return false, nil
}
