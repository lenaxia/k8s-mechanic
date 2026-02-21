package k8sgpt_test

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
	"github.com/lenaxia/k8s-mendabot/internal/provider/k8sgpt"
)

const integrationNamespace = "default"

func integrationCfg() config.Config {
	return config.Config{
		AgentNamespace:           integrationNamespace,
		MaxConcurrentJobs:        10,
		RemediationJobTTLSeconds: 604800,
		GitOpsRepo:               "org/repo",
		GitOpsManifestRoot:       "deploy",
		AgentImage:               "mendabot-agent:test",
		AgentSA:                  "mendabot-agent",
	}
}

func newSourceReconciler() *provider.SourceProviderReconciler {
	return &provider.SourceProviderReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Cfg:      integrationCfg(),
		Provider: &k8sgpt.K8sGPTProvider{},
	}
}

func eventually(t *testing.T, condition func() bool, timeout, interval time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatal("condition not met within timeout")
}

func newTestResult(name, namespace string, errors []v1alpha1.Failure) *v1alpha1.Result {
	return &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ResultSpec{
			Kind:         "Pod",
			Name:         name,
			ParentObject: "my-deployment",
			Error:        errors,
			Details:      "test finding details",
		},
	}
}

// TestSourceProviderReconciler_CreatesRemediationJob verifies: New Result →
// RemediationJob created with sourceType="k8sgpt".
func TestSourceProviderReconciler_CreatesRemediationJob(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	result := newTestResult("result-creates-rjob", integrationNamespace, []v1alpha1.Failure{
		{Text: "CrashLoopBackOff"},
	})
	if err := k8sClient.Create(ctx, result); err != nil {
		t.Fatalf("create Result: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, result)
	})

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: result.Name, Namespace: result.Namespace}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	eventually(t, func() bool {
		if err := k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace)); err != nil {
			return false
		}
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name &&
				rjobList.Items[i].Spec.SourceType == v1alpha1.SourceTypeK8sGPT {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var found *v1alpha1.RemediationJob
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
			found = &rjobList.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected RemediationJob to be created")
	}
	if found.Spec.SourceType != v1alpha1.SourceTypeK8sGPT {
		t.Errorf("sourceType = %q, want %q", found.Spec.SourceType, v1alpha1.SourceTypeK8sGPT)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, found)
	})
}

// TestSourceProviderReconciler_DuplicateFingerprint_Skips verifies: Same fingerprint →
// no second RemediationJob created.
func TestSourceProviderReconciler_DuplicateFingerprint_Skips(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	result := newTestResult("result-dedup", integrationNamespace, []v1alpha1.Failure{
		{Text: "ImagePullBackOff"},
	})
	if err := k8sClient.Create(ctx, result); err != nil {
		t.Fatalf("create Result: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, result)
	})

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: result.Name, Namespace: result.Namespace}}

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	eventually(t, func() bool {
		_ = k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace))
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	t.Cleanup(func() {
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
				_ = k8sClient.Delete(ctx, &rjobList.Items[i])
			}
		}
	})

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	var rjobList2 v1alpha1.RemediationJobList
	if err := k8sClient.List(ctx, &rjobList2, client.InNamespace(integrationNamespace)); err != nil {
		t.Fatalf("list RemediationJobs: %v", err)
	}
	count := 0
	for i := range rjobList2.Items {
		if rjobList2.Items[i].Spec.SourceResultRef.Name == result.Name {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 RemediationJob for duplicate fingerprint, got %d", count)
	}
}

// TestSourceProviderReconciler_FailedPhase_ReDispatches verifies: Existing Failed
// RemediationJob → new one created.
func TestSourceProviderReconciler_FailedPhase_ReDispatches(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	result := newTestResult("result-failed-redispatch", integrationNamespace, []v1alpha1.Failure{
		{Text: "OOMKilled"},
	})
	if err := k8sClient.Create(ctx, result); err != nil {
		t.Fatalf("create Result: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, result)
	})

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: result.Name, Namespace: result.Namespace}}

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	eventually(t, func() bool {
		_ = k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace))
		for i := range rjobList.Items {
			if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var firstRJob *v1alpha1.RemediationJob
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
			firstRJob = &rjobList.Items[i]
			break
		}
	}
	if firstRJob == nil {
		t.Fatal("expected first RemediationJob to be created")
	}

	rjobCopy := firstRJob.DeepCopyObject().(*v1alpha1.RemediationJob)
	firstRJob.Status.Phase = v1alpha1.PhaseFailed
	if err := k8sClient.Status().Patch(ctx, firstRJob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status to Failed: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, firstRJob)
	})

	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("second Reconcile (after Failed): %v", err)
	}

	var rjobList2 v1alpha1.RemediationJobList
	eventually(t, func() bool {
		if err := k8sClient.List(ctx, &rjobList2, client.InNamespace(integrationNamespace)); err != nil {
			return false
		}
		for i := range rjobList2.Items {
			if rjobList2.Items[i].Spec.SourceResultRef.Name == result.Name &&
				rjobList2.Items[i].Status.Phase != v1alpha1.PhaseFailed {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	var newRJob *v1alpha1.RemediationJob
	for i := range rjobList2.Items {
		if rjobList2.Items[i].Spec.SourceResultRef.Name == result.Name &&
			rjobList2.Items[i].Status.Phase != v1alpha1.PhaseFailed {
			newRJob = &rjobList2.Items[i]
			t.Cleanup(func() {
				_ = k8sClient.Delete(ctx, newRJob)
			})
			break
		}
	}
	if newRJob == nil {
		t.Error("expected a new non-Failed RemediationJob after re-dispatch")
	}
}

// TestSourceProviderReconciler_NoErrors_Skipped verifies: Result with no errors →
// ExtractFinding returns nil, nil → no RemediationJob created.
func TestSourceProviderReconciler_NoErrors_Skipped(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()
	result := newTestResult("result-no-errors", integrationNamespace, nil)
	if err := k8sClient.Create(ctx, result); err != nil {
		t.Fatalf("create Result: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, result)
	})

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: result.Name, Namespace: result.Namespace}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var rjobList v1alpha1.RemediationJobList
	if err := k8sClient.List(ctx, &rjobList, client.InNamespace(integrationNamespace)); err != nil {
		t.Fatalf("list RemediationJobs: %v", err)
	}
	for i := range rjobList.Items {
		if rjobList.Items[i].Spec.SourceResultRef.Name == result.Name {
			t.Errorf("expected no RemediationJob for no-error Result, found %q", rjobList.Items[i].Name)
		}
	}
}

// TestSourceProviderReconciler_ResultDeleted_CancelsPending verifies: Result deleted →
// Pending RemediationJob deleted.
func TestSourceProviderReconciler_ResultDeleted_CancelsPending(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	fp := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: integrationNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeK8sGPT,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "result-deleted-pending",
				Namespace: integrationNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-abc",
				Namespace:    integrationNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}
	if err := k8sClient.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, rjob)
	})

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      "result-deleted-pending",
		Namespace: integrationNamespace,
	}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile (NotFound): %v", err)
	}

	eventually(t, func() bool {
		var fetched v1alpha1.RemediationJob
		err := k8sClient.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationNamespace}, &fetched)
		return err != nil
	}, 5*time.Second, 100*time.Millisecond)
}

// TestSourceProviderReconciler_ResultDeleted_CancelsDispatched verifies: Result deleted →
// Dispatched RemediationJob deleted.
func TestSourceProviderReconciler_ResultDeleted_CancelsDispatched(t *testing.T) {
	if !suiteReady {
		t.Skip("envtest not available: KUBEBUILDER_ASSETS not set")
	}
	ctx := context.Background()

	fp := "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: integrationNamespace,
			Labels: map[string]string{
				"remediation.mendabot.io/fingerprint": fp[:12],
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/fingerprint-full": fp,
			},
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceType:  v1alpha1.SourceTypeK8sGPT,
			SinkType:    "github",
			Fingerprint: fp,
			SourceResultRef: v1alpha1.ResultRef{
				Name:      "result-deleted-dispatched",
				Namespace: integrationNamespace,
			},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod-def",
				Namespace:    integrationNamespace,
				ParentObject: "my-deploy",
			},
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
		},
	}
	if err := k8sClient.Create(ctx, rjob); err != nil {
		t.Fatalf("create RemediationJob: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, rjob)
	})

	rjobCopy := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob.Status.Phase = v1alpha1.PhaseDispatched
	if err := k8sClient.Status().Patch(ctx, rjob, client.MergeFrom(rjobCopy)); err != nil {
		t.Fatalf("patch status to Dispatched: %v", err)
	}

	rec := newSourceReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      "result-deleted-dispatched",
		Namespace: integrationNamespace,
	}}
	if _, err := rec.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile (NotFound): %v", err)
	}

	eventually(t, func() bool {
		var fetched v1alpha1.RemediationJob
		err := k8sClient.Get(ctx, types.NamespacedName{Name: rjob.Name, Namespace: integrationNamespace}, &fetched)
		return err != nil
	}, 5*time.Second, 100*time.Millisecond)
}
