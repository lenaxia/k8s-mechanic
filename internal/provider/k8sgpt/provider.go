package k8sgpt

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: K8sGPTProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*K8sGPTProvider)(nil)

// K8sGPTProvider watches k8sgpt Result CRDs and extracts Findings from them.
type K8sGPTProvider struct{}

// ProviderName returns the stable identifier for this provider.
func (p *K8sGPTProvider) ProviderName() string { return v1alpha1.SourceTypeK8sGPT }

// ObjectType returns the runtime.Object type this provider watches.
func (p *K8sGPTProvider) ObjectType() client.Object { return &v1alpha1.Result{} }

// ExtractFinding converts a watched Result object into a Finding.
// Returns (nil, nil) if the Result has no errors (skip).
// Returns (nil, err) if the object is the wrong type.
func (p *K8sGPTProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	result, ok := obj.(*v1alpha1.Result)
	if !ok {
		return nil, fmt.Errorf("K8sGPTProvider: expected *Result, got %T", obj)
	}
	if len(result.Spec.Error) == 0 {
		return nil, nil
	}

	redacted := make([]v1alpha1.Failure, len(result.Spec.Error))
	for i, f := range result.Spec.Error {
		redacted[i] = v1alpha1.Failure{Text: f.Text}
	}
	errorsJSON, err := json.Marshal(redacted)
	if err != nil {
		return nil, fmt.Errorf("K8sGPTProvider: serialising errors: %w", err)
	}

	return &domain.Finding{
		Kind:         result.Spec.Kind,
		Name:         result.Spec.Name,
		Namespace:    result.Namespace,
		ParentObject: result.Spec.ParentObject,
		Errors:       string(errorsJSON),
		Details:      result.Spec.Details,
		SourceRef: domain.SourceRef{
			APIVersion: "core.k8sgpt.ai/v1alpha1",
			Kind:       "Result",
			Name:       result.Name,
			Namespace:  result.Namespace,
		},
	}, nil
}
