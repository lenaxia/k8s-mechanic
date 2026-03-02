package llm

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VertexChecker is the readiness checker for GCP Vertex AI LLM endpoints.
// It validates the vertex-credentials Secret and probes the Vertex AI API.
//
// This checker is a stub — it is not yet implemented and is not wired into
// the watcher. It is retained as an extension point for future use.
type VertexChecker struct {
	client    client.Client
	namespace string
}

// NewVertexChecker returns a VertexChecker that reads secrets from namespace.
func NewVertexChecker(c client.Client, namespace string) *VertexChecker {
	return &VertexChecker{client: c, namespace: namespace}
}

func (v *VertexChecker) Name() string { return "llm/vertex" }

// Check is not yet implemented. It always returns an error.
func (v *VertexChecker) Check(_ context.Context) error {
	return errors.New("llm/vertex: checker not yet implemented")
}
