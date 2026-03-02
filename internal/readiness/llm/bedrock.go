package llm

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BedrockChecker is the readiness checker for AWS Bedrock LLM endpoints.
// It validates the bedrock-credentials Secret and probes the Bedrock API.
//
// This checker is a stub — it is not yet implemented and is not wired into
// the watcher. It is retained as an extension point for future use.
type BedrockChecker struct {
	client    client.Client
	namespace string
}

// NewBedrockChecker returns a BedrockChecker that reads secrets from namespace.
func NewBedrockChecker(c client.Client, namespace string) *BedrockChecker {
	return &BedrockChecker{client: c, namespace: namespace}
}

func (b *BedrockChecker) Name() string { return "llm/bedrock" }

// Check is not yet implemented. It always returns an error.
func (b *BedrockChecker) Check(_ context.Context) error {
	return errors.New("llm/bedrock: checker not yet implemented")
}
