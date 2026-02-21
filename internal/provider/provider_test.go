package provider_test

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/provider"
)

// fakeSourceProvider is a controllable domain.SourceProvider for unit tests.
type fakeSourceProvider struct {
	name       string
	objectType client.Object
	finding    *domain.Finding
	findErr    error
	fp         string
	fpErr      error
}

func (f *fakeSourceProvider) ProviderName() string      { return f.name }
func (f *fakeSourceProvider) ObjectType() client.Object { return f.objectType }
func (f *fakeSourceProvider) ExtractFinding(_ client.Object) (*domain.Finding, error) {
	return f.finding, f.findErr
}
func (f *fakeSourceProvider) Fingerprint(_ *domain.Finding) (string, error) { return f.fp, f.fpErr }

var _ domain.SourceProvider = (*fakeSourceProvider)(nil)

const agentNamespace = "mendabot"

func newTestScheme() *runtime.Scheme {
	s := v1alpha1.NewScheme()
	return s
}

func newTestClient(objs ...client.Object) client.Client {
	s := newTestScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&v1alpha1.RemediationJob{}).
		WithObjects(objs...).
		Build()
}

func newTestReconciler(p *fakeSourceProvider, c client.Client) *provider.SourceProviderReconciler {
	return &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace},
		Provider: p,
	}
}

func makeWatchedObject(name, namespace string) *v1alpha1.Result {
	return &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ResultSpec{
			Kind:         "Pod",
			ParentObject: "my-deploy",
			Error:        []v1alpha1.Failure{{Text: "CrashLoopBackOff"}},
		},
	}
}

func reqFor(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

// TestSourceProviderReconciler_CallsExtractFinding verifies ExtractFinding is invoked.
func TestSourceProviderReconciler_CallsExtractFinding(t *testing.T) {
	called := false
	p := &fakeSourceProvider{
		name:       "fake",
		objectType: &v1alpha1.Result{},
		findErr:    nil,
	}
	// Override to track calls
	p.finding = nil // nil finding → skip, but still calls ExtractFinding

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)

	// Use a custom provider that records calls
	trackingProvider := &trackingFakeProvider{inner: p}
	r := &provider.SourceProviderReconciler{
		Client:   c,
		Scheme:   newTestScheme(),
		Cfg:      config.Config{AgentNamespace: agentNamespace},
		Provider: trackingProvider,
	}

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !trackingProvider.extractCalled {
		t.Error("expected ExtractFinding to be called")
	}
	_ = called
}

type trackingFakeProvider struct {
	inner         *fakeSourceProvider
	extractCalled bool
}

func (t *trackingFakeProvider) ProviderName() string      { return t.inner.name }
func (t *trackingFakeProvider) ObjectType() client.Object { return t.inner.objectType }
func (t *trackingFakeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	t.extractCalled = true
	return t.inner.ExtractFinding(obj)
}
func (t *trackingFakeProvider) Fingerprint(f *domain.Finding) (string, error) {
	return t.inner.fp, nil
}

// TestSourceProviderReconciler_SkipsOnNilFinding verifies no RemediationJob is created when
// ExtractFinding returns nil, nil.
func TestSourceProviderReconciler_SkipsOnNilFinding(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "fake",
		objectType: &v1alpha1.Result{},
		finding:    nil,
		findErr:    nil,
	}
	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_CreatesRemediationJob verifies a RemediationJob is created
// with correct fields for a valid finding.
func TestSourceProviderReconciler_CreatesRemediationJob(t *testing.T) {
	const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
		finding: &domain.Finding{
			Kind:         "Pod",
			Name:         "pod-abc",
			Namespace:    "default",
			ParentObject: "my-deploy",
			Errors:       `[{"text":"CrashLoopBackOff"}]`,
			Details:      "Pod is crash looping",
		},
		fp: fp,
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 RemediationJob, got %d", len(list.Items))
	}

	rjob := list.Items[0]
	expectedName := "mendabot-" + fp[:12]
	if rjob.Name != expectedName {
		t.Errorf("name = %q, want %q", rjob.Name, expectedName)
	}
	if rjob.Spec.Fingerprint != fp {
		t.Errorf("fingerprint = %q, want %q", rjob.Spec.Fingerprint, fp)
	}
	if rjob.Spec.SourceType != "k8sgpt" {
		t.Errorf("sourceType = %q, want %q", rjob.Spec.SourceType, "k8sgpt")
	}
	if rjob.Spec.SourceResultRef.Name != "r1" {
		t.Errorf("sourceResultRef.Name = %q, want %q", rjob.Spec.SourceResultRef.Name, "r1")
	}
	if rjob.Spec.SourceResultRef.Namespace != "default" {
		t.Errorf("sourceResultRef.Namespace = %q, want %q", rjob.Spec.SourceResultRef.Namespace, "default")
	}
	if rjob.Labels["remediation.mendabot.io/fingerprint"] != fp[:12] {
		t.Errorf("fingerprint label = %q, want %q", rjob.Labels["remediation.mendabot.io/fingerprint"], fp[:12])
	}
	if rjob.Annotations["remediation.mendabot.io/fingerprint-full"] != fp {
		t.Errorf("fingerprint-full annotation = %q, want %q", rjob.Annotations["remediation.mendabot.io/fingerprint-full"], fp)
	}
	if rjob.Spec.Finding.Kind != "Pod" {
		t.Errorf("finding.kind = %q, want %q", rjob.Spec.Finding.Kind, "Pod")
	}
}

// TestSourceProviderReconciler_SkipsDuplicateFingerprint verifies no second RemediationJob is
// created when a non-Failed one with the same fingerprint already exists.
func TestSourceProviderReconciler_SkipsDuplicateFingerprint(t *testing.T) {
	const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
		finding: &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
			Errors: `[{"text":"error"}]`,
		},
		fp: fp,
	}

	existing := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: agentNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/fingerprint": fp[:12]},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: fp,
			SourceType:  "k8sgpt",
		},
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj, existing)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected exactly 1 RemediationJob (existing), got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_ReDispatchesFailedRemediationJob verifies a new RemediationJob
// is created when the existing one has phase Failed. The Failed one is deleted first, then a
// new one with the standard name is created.
func TestSourceProviderReconciler_ReDispatchesFailedRemediationJob(t *testing.T) {
	const fp = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
		finding: &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
			Errors: `[{"text":"error"}]`,
		},
		fp: fp,
	}

	// Existing Failed RemediationJob with same fingerprint and standard name.
	// The reconciler should delete it and create a new one with the same name.
	failedRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-" + fp[:12],
			Namespace: agentNamespace,
			Labels:    map[string]string{"remediation.mendabot.io/fingerprint": fp[:12]},
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint:        fp,
			SourceType:         "k8sgpt",
			SinkType:           "github",
			GitOpsRepo:         "org/repo",
			GitOpsManifestRoot: "deploy",
			AgentImage:         "mendabot-agent:test",
			AgentSA:            "mendabot-agent",
			SourceResultRef:    v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
			Finding: v1alpha1.FindingSpec{
				Kind:         "Pod",
				Name:         "pod",
				Namespace:    "default",
				ParentObject: "my-deploy",
			},
		},
		Status: v1alpha1.RemediationJobStatus{
			Phase: v1alpha1.PhaseFailed,
		},
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj, failedRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	// Failed one deleted, new one created — net result is 1.
	if len(list.Items) != 1 {
		t.Errorf("expected 1 RemediationJob (failed deleted, new created), got %d", len(list.Items))
	}
	if list.Items[0].Status.Phase == v1alpha1.PhaseFailed {
		t.Error("expected new RemediationJob not to be in Failed phase")
	}
}

// TestSourceProviderReconciler_NotFound_DeletesPendingRJobs verifies that when the watched
// object is not found, any Pending/Dispatched RemediationJobs for that source ref are deleted.
func TestSourceProviderReconciler_NotFound_DeletesPendingRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
	}

	pendingRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-pending",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhasePending},
	}

	// No Result object — it's been deleted
	c := newTestClient(pendingRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_NotFound_DeletesDispatchedRJobs verifies Dispatched jobs are
// also cancelled when the source Result is deleted.
func TestSourceProviderReconciler_NotFound_DeletesDispatchedRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
	}

	dispatchedRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-dispatched",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhaseDispatched},
	}

	c := newTestClient(dispatchedRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_NotFound_DeletesRunningRJobs verifies Running jobs are
// also cancelled when the source Result is deleted.
func TestSourceProviderReconciler_NotFound_DeletesRunningRJobs(t *testing.T) {
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
	}

	runningRJob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-running",
			Namespace: agentNamespace,
		},
		Spec: v1alpha1.RemediationJobSpec{
			SourceResultRef: v1alpha1.ResultRef{Name: "r1", Namespace: "default"},
		},
		Status: v1alpha1.RemediationJobStatus{Phase: v1alpha1.PhaseRunning},
	}

	c := newTestClient(runningRJob)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var list v1alpha1.RemediationJobList
	if err := c.List(context.Background(), &list, client.InNamespace(agentNamespace)); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 RemediationJobs after source deleted, got %d", len(list.Items))
	}
}

// TestSourceProviderReconciler_FingerprintError_ReturnsError verifies that a Fingerprint
// error is propagated as a reconciler error.
func TestSourceProviderReconciler_FingerprintError_ReturnsError(t *testing.T) {
	fpErr := fmt.Errorf("hash write failed")
	p := &fakeSourceProvider{
		name:       "k8sgpt",
		objectType: &v1alpha1.Result{},
		finding: &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "my-deploy",
			Errors: `[{"text":"error"}]`,
		},
		fpErr: fpErr,
	}

	obj := makeWatchedObject("r1", "default")
	c := newTestClient(obj)
	r := newTestReconciler(p, c)

	_, err := r.Reconcile(context.Background(), reqFor("r1", "default"))
	if err == nil {
		t.Error("expected error from Fingerprint failure, got nil")
	}
}
