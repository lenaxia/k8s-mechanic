package k8sgpt

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

func makeResult(ns, kind, name, parentObject string, errors []v1alpha1.Failure) *v1alpha1.Result {
	return &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1alpha1.ResultSpec{
			Kind:         kind,
			Name:         name,
			ParentObject: parentObject,
			Error:        errors,
		},
	}
}

func TestFingerprintFor_SameParentDifferentPods(t *testing.T) {
	errs := []v1alpha1.Failure{{Text: "CrashLoopBackOff"}}
	fp1 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: errs, Name: "pod-aaa"})
	fp2 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: errs, Name: "pod-bbb"})
	if fp1 != fp2 {
		t.Errorf("expected same fingerprint for same parent+errors, different pod name; got %q vs %q", fp1, fp2)
	}
}

func TestFingerprintFor_DifferentErrors(t *testing.T) {
	spec1 := v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: []v1alpha1.Failure{{Text: "error A"}}}
	spec2 := v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: []v1alpha1.Failure{{Text: "error B"}}}
	fp1 := fingerprintFor("default", spec1)
	fp2 := fingerprintFor("default", spec2)
	if fp1 == fp2 {
		t.Errorf("expected different fingerprints for different errors; both got %q", fp1)
	}
}

func TestFingerprintFor_ErrorOrderIndependent(t *testing.T) {
	errs1 := []v1alpha1.Failure{{Text: "alpha"}, {Text: "beta"}}
	errs2 := []v1alpha1.Failure{{Text: "beta"}, {Text: "alpha"}}
	fp1 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: errs1})
	fp2 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: errs2})
	if fp1 != fp2 {
		t.Errorf("expected same fingerprint regardless of error order; got %q vs %q", fp1, fp2)
	}
}

func TestFingerprintFor_DifferentParents(t *testing.T) {
	errs := []v1alpha1.Failure{{Text: "OOMKilled"}}
	fp1 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "deploy-A", Error: errs})
	fp2 := fingerprintFor("default", v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "deploy-B", Error: errs})
	if fp1 == fp2 {
		t.Errorf("expected different fingerprints for different parents; both got %q", fp1)
	}
}

func TestFingerprintFor_DifferentNamespaces(t *testing.T) {
	errs := []v1alpha1.Failure{{Text: "ImagePullBackOff"}}
	spec := v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: errs}
	fp1 := fingerprintFor("namespace-A", spec)
	fp2 := fingerprintFor("namespace-B", spec)
	if fp1 == fp2 {
		t.Errorf("expected different fingerprints for different namespaces; both got %q", fp1)
	}
}

func TestFingerprintFor_EmptyErrors(t *testing.T) {
	specNil := v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: nil}
	specEmpty := v1alpha1.ResultSpec{Kind: "Pod", ParentObject: "my-deploy", Error: []v1alpha1.Failure{}}
	fp1 := fingerprintFor("default", specNil)
	fp2 := fingerprintFor("default", specEmpty)
	if fp1 != fp2 {
		t.Errorf("expected same fingerprint for nil vs empty errors; got %q vs %q", fp1, fp2)
	}
}

func TestFingerprintFor_Deterministic(t *testing.T) {
	errs := []v1alpha1.Failure{{Text: "connection refused"}}
	spec := v1alpha1.ResultSpec{Kind: "Deployment", ParentObject: "my-deploy", Error: errs}
	fp1 := fingerprintFor("default", spec)
	fp2 := fingerprintFor("default", spec)
	if fp1 != fp2 {
		t.Errorf("expected deterministic output; got %q vs %q", fp1, fp2)
	}
}

func TestFingerprintEquivalence(t *testing.T) {
	provider := &K8sGPTProvider{}

	tests := []struct {
		name   string
		result *v1alpha1.Result
	}{
		{
			name: "basic_errors",
			result: makeResult("default", "Pod", "pod-abc", "my-deploy", []v1alpha1.Failure{
				{Text: "CrashLoopBackOff"},
				{Text: "back-off restarting failed container"},
			}),
		},
		{
			name: "html_special_chars",
			result: makeResult("production", "Deployment", "my-deploy", "my-deploy", []v1alpha1.Failure{
				{Text: "error: <nil> pointer & invalid > comparison"},
			}),
		},
		{
			name: "single_error",
			result: makeResult("kube-system", "Pod", "coredns-xyz", "coredns", []v1alpha1.Failure{
				{Text: "OOMKilled"},
			}),
		},
		{
			name: "multiple_errors_ordered",
			result: makeResult("default", "Pod", "web-pod", "web-deploy", []v1alpha1.Failure{
				{Text: "alpha"},
				{Text: "beta"},
				{Text: "gamma"},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpDirect := fingerprintFor(tt.result.Namespace, tt.result.Spec)

			finding, err := provider.ExtractFinding(tt.result)
			if err != nil {
				t.Fatalf("ExtractFinding returned error: %v", err)
			}
			if finding == nil {
				t.Fatal("ExtractFinding returned nil finding")
			}

			fpViaProvider := provider.Fingerprint(finding)

			if fpDirect != fpViaProvider {
				t.Errorf("fingerprint mismatch:\n  fingerprintFor: %q\n  Fingerprint():  %q",
					fpDirect, fpViaProvider)
			}
		})
	}
}
