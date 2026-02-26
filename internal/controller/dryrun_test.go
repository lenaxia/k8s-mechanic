package controller_test

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/controller"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newDryRunRJobWithJob(
	rjobName, fp string,
	rjobPhase v1alpha1.RemediationJobPhase,
	jobAnnotations map[string]string,
) (*v1alpha1.RemediationJob, *batchv1.Job) {
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rjobName,
			Namespace: testNamespace,
			UID:       types.UID("uid-" + rjobName),
		},
		Spec: v1alpha1.RemediationJobSpec{Fingerprint: fp},
	}
	rjob.Status.Phase = rjobPhase

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "mendabot-agent-" + fp[:12],
			Namespace:   testNamespace,
			Labels:      map[string]string{"remediation.mendabot.io/remediation-job": rjobName},
			Annotations: jobAnnotations,
		},
		Spec:   batchv1.JobSpec{BackoffLimit: ptr(int32(1))},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	return rjob, job
}

// newDryRunCM creates the ConfigMap that emit_dry_run_report() would write.
// cmName must match controller.dryRunCMName(fp).
func newDryRunCM(cmName, namespace, report, patch string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"report": report,
			"patch":  patch,
		},
	}
}

// dryRunCMName mirrors the private function in the controller package so tests
// can derive the expected ConfigMap name from a fingerprint.
func dryRunCMName(fp string) string {
	if len(fp) > 12 {
		fp = fp[:12]
	}
	return "mendabot-dryrun-" + fp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_DryRunSucceeded_ReportAndPatchStored verifies the happy path:
// ConfigMap present with report+patch → both appear in status.message; CM deleted.
func TestReconcile_DryRunSucceeded_ReportAndPatchStored(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-full", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cm := newDryRunCM(dryRunCMName(fp), testNamespace,
		"## Root Cause\nImagePullBackOff — image not found.",
		"diff --git a/foo.yaml b/foo.yaml\n--- a/foo.yaml\n+++ b/foo.yaml\n@@ -1 +1 @@\n-old\n+new",
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-full", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-full", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseSucceeded {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseSucceeded)
	}
	if !strings.Contains(updated.Status.Message, "ImagePullBackOff") {
		t.Errorf("Message = %q — want report content", updated.Status.Message)
	}
	if !strings.Contains(updated.Status.Message, "PROPOSED PATCH") {
		t.Errorf("Message = %q — want patch section", updated.Status.Message)
	}
	if !strings.Contains(updated.Status.Message, "+new") {
		t.Errorf("Message = %q — want patch content", updated.Status.Message)
	}

	// ConfigMap must be deleted after reading.
	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: dryRunCMName(fp), Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Error("expected ConfigMap to be deleted after reading, but it still exists")
	}
}

// TestReconcile_DryRunSucceeded_ReportOnlyNoPatch verifies that when the patch
// key is empty, the PROPOSED PATCH section is omitted from status.message.
func TestReconcile_DryRunSucceeded_ReportOnlyNoPatch(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nopatch", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cm := newDryRunCM(dryRunCMName(fp), testNamespace,
		"## Root Cause\nThe image tag was wrong.",
		"", // no patch
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nopatch", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nopatch", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.Contains(updated.Status.Message, "image tag was wrong") {
		t.Errorf("Message = %q — want report content", updated.Status.Message)
	}
	if strings.Contains(updated.Status.Message, "PROPOSED PATCH") {
		t.Errorf("Message = %q — must NOT contain patch section when patch is empty", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_CMNotFound verifies that when the ConfigMap is
// absent (agent crashed before writing it), Message starts with
// "dry-run report unavailable".
func TestReconcile_DryRunSucceeded_CMNotFound(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-nocm", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	// No ConfigMap created.
	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-nocm", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-nocm", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.HasPrefix(updated.Status.Message, "dry-run report unavailable") {
		t.Errorf("Message = %q, want prefix \"dry-run report unavailable\"", updated.Status.Message)
	}
}

// TestReconcile_NoDryRun_MessageNotPopulated verifies that a succeeded Job
// without the dry-run annotation leaves Message empty.
func TestReconcile_NoDryRun_MessageNotPopulated(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-nodryrun", fp, v1alpha1.PhaseDispatched,
		nil, // no annotations
	)

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-nodryrun", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Message != "" {
		t.Errorf("Message = %q, want empty string for non-dry-run succeeded job", updated.Status.Message)
	}
}

// TestReconcile_DryRunSucceeded_MessageAlreadySet verifies that when
// rjob.Status.Message is already set, the reconciler does not overwrite it
// and does not attempt to read the ConfigMap again.
func TestReconcile_DryRunSucceeded_MessageAlreadySet(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-alreadyset", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)
	rjob.Status.Message = "existing report"

	// A CM exists with different content — if idempotency guard fires, CM is
	// NOT read and Message stays "existing report".
	cm := newDryRunCM(dryRunCMName(fp), testNamespace, "NEW CONTENT", "")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-alreadyset", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-alreadyset", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if updated.Status.Message != "existing report" {
		t.Errorf("Message = %q, want \"existing report\" — idempotency guard must prevent overwrite", updated.Status.Message)
	}
	if updated.Status.Phase != v1alpha1.PhaseSucceeded {
		t.Errorf("Phase = %q, want %q", updated.Status.Phase, v1alpha1.PhaseSucceeded)
	}

	// CM must NOT have been deleted (guard fired before fetchDryRunReport).
	var remaining corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: dryRunCMName(fp), Namespace: testNamespace}, &remaining); err != nil {
		t.Error("expected ConfigMap to still exist when idempotency guard fired, but it was deleted")
	}
}

// TestReconcile_DryRunSucceeded_CMNameDerivedFromFingerprint verifies that the
// controller constructs the ConfigMap name as "mendabot-dryrun-<fp[:12]>", which
// must match what emit_dry_run_report() writes.
func TestReconcile_DryRunSucceeded_CMNameDerivedFromFingerprint(t *testing.T) {
	const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-cmname", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	// Use a well-known CM name derived from the fingerprint prefix.
	expectedCMName := "mendabot-dryrun-aabbccddeeff"
	cm := newDryRunCM(expectedCMName, testNamespace, "root cause found", "")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated v1alpha1.RemediationJob
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace}, &updated); err != nil {
		t.Fatalf("get rjob: %v", err)
	}
	if !strings.Contains(updated.Status.Message, "root cause found") {
		t.Errorf("Message = %q — expected report content; CM name derivation may be wrong", updated.Status.Message)
	}

	// Verify the CM was consumed (deleted).
	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: expectedCMName, Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Error("expected ConfigMap to be deleted after reading")
	}

	// Also confirm a CM with the wrong name would NOT have been consumed.
	wrongName := "mendabot-dryrun-wrongname12"
	wrongCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: wrongName, Namespace: testNamespace},
		Data:       map[string]string{"report": "should not appear"},
	}
	c2 := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, wrongCM).
		Build()
	// Reset rjob phase for second run.
	rjob2 := rjob.DeepCopyObject().(*v1alpha1.RemediationJob)
	rjob2.Status.Message = ""
	if patchErr := c2.Status().Update(context.Background(), rjob2); patchErr != nil {
		t.Fatalf("reset rjob: %v", patchErr)
	}
	r2 := &controller.RemediationJobReconciler{
		Client:     c2,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}
	_, _ = r2.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace},
	})
	var updated2 v1alpha1.RemediationJob
	if err := c2.Get(context.Background(), types.NamespacedName{Name: "test-dryrun-cmname", Namespace: testNamespace}, &updated2); err != nil {
		t.Fatalf("get rjob2: %v", err)
	}
	if strings.Contains(updated2.Status.Message, "should not appear") {
		t.Errorf("wrong CM name was used; message = %q", updated2.Status.Message)
	}
	// wrongCM must still exist (was never found by the controller).
	var stillThere corev1.ConfigMap
	if err := c2.Get(context.Background(), types.NamespacedName{Name: wrongName, Namespace: testNamespace}, &stillThere); err != nil {
		t.Error("wrong-named CM was unexpectedly deleted")
	}
}

// TestReconcile_DryRunSucceeded_CMDeletedAfterRead verifies the controller
// performs a best-effort delete after reading, so stale CMs do not accumulate.
// (This is implicitly covered by TestReconcile_DryRunSucceeded_ReportAndPatchStored
// but is kept as an explicit, focused test.)
func TestReconcile_DryRunSucceeded_CMDeletedAfterRead(t *testing.T) {
	const fp = "abcdefghijklmnopqrstuvwxyz012345abcdefghijklmnopqrstuvwxyz012345"
	rjob, job := newDryRunRJobWithJob(
		"test-dryrun-cmdelete", fp, v1alpha1.PhaseDispatched,
		map[string]string{"mendabot.io/dry-run": "true"},
	)

	cmName := dryRunCMName(fp)
	cm := newDryRunCM(cmName, testNamespace, "some report", "some patch")

	s := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm).
		Build()

	r := &controller.RemediationJobReconciler{
		Client:     c,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmdelete", Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var remaining corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: testNamespace}, &remaining)
	if err == nil {
		t.Errorf("ConfigMap %q still exists after reconcile — expected deletion", cmName)
	}

	// Confirm other ConfigMaps in the namespace are not affected.
	unrelated := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated-cm", Namespace: testNamespace},
		Data:       map[string]string{"key": "value"},
	}
	c2 := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(rjob, job, cm, unrelated).
		Build()
	r2 := &controller.RemediationJobReconciler{
		Client:     c2,
		Scheme:     s,
		JobBuilder: &fakeJobBuilder{},
		Cfg:        defaultCfg(),
	}
	_, _ = r2.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-dryrun-cmdelete", Namespace: testNamespace},
	})
	var unrelatedCM corev1.ConfigMap
	if err := c2.Get(context.Background(), types.NamespacedName{Name: "unrelated-cm", Namespace: testNamespace}, &unrelatedCM); err != nil {
		t.Error("unrelated ConfigMap was unexpectedly deleted")
	}

	_ = client.IgnoreNotFound(err)
}
