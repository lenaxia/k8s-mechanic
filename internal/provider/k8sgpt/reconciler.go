package k8sgpt

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

// fingerprintFor computes a stable SHA256 fingerprint from a Result's namespace and spec.
// It is keyed on namespace + kind + parentObject + sorted error texts, so multiple pods
// from the same Deployment produce one fingerprint. Uses SetEscapeHTML(false) so that
// error texts containing <, >, & hash identically to K8sGPTProvider.Fingerprint().
func fingerprintFor(namespace string, spec v1alpha1.ResultSpec) string {
	texts := make([]string, 0, len(spec.Error))
	for _, f := range spec.Error {
		texts = append(texts, f.Text)
	}
	sort.Strings(texts)

	payload := struct {
		Namespace    string   `json:"namespace"`
		Kind         string   `json:"kind"`
		ParentObject string   `json:"parentObject"`
		ErrorTexts   []string `json:"errorTexts"`
	}{
		Namespace:    namespace,
		Kind:         spec.Kind,
		ParentObject: spec.ParentObject,
		ErrorTexts:   texts,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		panic(fmt.Sprintf("fingerprintFor: json.Encode failed: %v", err))
	}
	return fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
}
