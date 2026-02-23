package native

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestChainDepthCascadeSimulation simulates a cascade of mendabot job failures
// and verifies chain depth is tracked correctly without races.
func TestChainDepthCascadeSimulation(t *testing.T) {
	// Create scheme with all required types
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = v1alpha1.AddRemediationToScheme(s)

	cfg := config.Config{
		GitOpsRepo:              "test/repo",
		GitOpsManifestRoot:      "manifests",
		AgentImage:              "test/image:latest",
		AgentNamespace:          "default",
		AgentSA:                 "default",
		SelfRemediationMaxDepth: 5, // Allow deeper cascade for test
	}

	// Step 1: Initial failure (non-mendabot job)
	t.Log("Step 1: Initial non-mendabot job failure")
	c := fake.NewClientBuilder().WithScheme(s).Build()
	p := NewJobProvider(c, cfg)

	job1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-job-1",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Failed: 3,
			Active: 0,
		},
	}

	finding1, err := p.ExtractFinding(job1)
	if err != nil {
		t.Fatalf("ExtractFinding job1: %v", err)
	}
	if finding1 == nil {
		t.Fatal("expected finding for job1, got nil")
	}
	if finding1.IsSelfRemediation {
		t.Error("finding1 should not be self-remediation")
	}
	if finding1.ChainDepth != 0 {
		t.Errorf("finding1.ChainDepth = %d, want 0", finding1.ChainDepth)
	}

	// Step 2: First mendabot job failure (remediation of job1)
	t.Log("Step 2: First mendabot job failure (depth 1)")

	// Create RemediationJob R1 for job1 (chain depth 0 in spec)
	rjob1 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remediation-1",
			Namespace: "default",
			UID:       "r1-uid",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 0, // Initial remediation
		},
	}

	// Job J1 is created by R1, fails
	job2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-abc123",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/chain-depth": "0", // From R1
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "remediation-1",
					UID:                "r1-uid",
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Active: 0,
		},
	}

	c = fake.NewClientBuilder().WithScheme(s).WithObjects(rjob1).Build()
	p = NewJobProvider(c, cfg)

	finding2, err := p.ExtractFinding(job2)
	if err != nil {
		t.Fatalf("ExtractFinding job2: %v", err)
	}
	if finding2 == nil {
		t.Fatal("expected finding for mendabot job failure, got nil")
	}
	if !finding2.IsSelfRemediation {
		t.Error("finding2 should be self-remediation")
	}
	if finding2.ChainDepth != 1 {
		t.Errorf("finding2.ChainDepth = %d, want 1 (0 from owner + 1)", finding2.ChainDepth)
	}

	// Step 3: Second mendabot job failure (remediation of job2)
	t.Log("Step 3: Second mendabot job failure (depth 2)")

	// Create RemediationJob R2 for job2 (chain depth 1 in spec)
	rjob2 := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remediation-2",
			Namespace: "default",
			UID:       "r2-uid",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 1, // From finding2
		},
	}

	// Job J2 is created by R2, fails
	job3 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mendabot-agent-def456",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot-watcher",
			},
			Annotations: map[string]string{
				"remediation.mendabot.io/chain-depth": "1", // From R2
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "remediation.mendabot.io/v1alpha1",
					Kind:               "RemediationJob",
					Name:               "remediation-2",
					UID:                "r2-uid",
					Controller:         ptr(true),
					BlockOwnerDeletion: ptr(true),
				},
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Active: 0,
		},
	}

	c = fake.NewClientBuilder().WithScheme(s).WithObjects(rjob2).Build()
	p = NewJobProvider(c, cfg)

	finding3, err := p.ExtractFinding(job3)
	if err != nil {
		t.Fatalf("ExtractFinding job3: %v", err)
	}
	if finding3 == nil {
		t.Fatal("expected finding for second mendabot job failure, got nil")
	}
	if !finding3.IsSelfRemediation {
		t.Error("finding3 should be self-remediation")
	}
	if finding3.ChainDepth != 2 {
		t.Errorf("finding3.ChainDepth = %d, want 2 (1 from owner + 1)", finding3.ChainDepth)
	}

	// Step 4: Verify max depth enforcement
	t.Log("Step 4: Max depth enforcement")

	cfg2 := config.Config{
		GitOpsRepo:              "test/repo",
		GitOpsManifestRoot:      "manifests",
		AgentImage:              "test/image:latest",
		AgentNamespace:          "default",
		AgentSA:                 "default",
		SelfRemediationMaxDepth: 1,
	}

	p2 := NewJobProvider(c, cfg2)
	finding4, err := p2.ExtractFinding(job3)
	if err != nil {
		t.Fatalf("ExtractFinding job3 with max depth 1: %v", err)
	}
	if finding4 != nil {
		t.Errorf("expected nil finding when chain depth (2) exceeds max depth (1), got %+v", finding4)
	}
}

// TestConcurrentRemediationJobUpdates simulates concurrent updates to parent RemediationJob
// while child jobs are being processed.
func TestConcurrentRemediationJobUpdates(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = v1alpha1.AddRemediationToScheme(s)

	cfg := config.Config{
		GitOpsRepo:              "test/repo",
		GitOpsManifestRoot:      "manifests",
		AgentImage:              "test/image:latest",
		AgentNamespace:          "default",
		AgentSA:                 "default",
		SelfRemediationMaxDepth: 3,
	}

	// Create parent RemediationJob
	rjob := &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "parent",
			Namespace:       "default",
			UID:             "parent-uid",
			ResourceVersion: "1",
		},
		Spec: v1alpha1.RemediationJobSpec{
			ChainDepth: 1,
		},
		Status: v1alpha1.RemediationJobStatus{
			Phase: v1alpha1.PhaseFailed,
		},
	}

	// Create a fake client that simulates concurrent updates
	// In real scenario, multiple reconcilers would have their own clients
	// reading the same resource. Our implementation should handle this.
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(rjob).Build()

	// Simulate multiple "threads" (goroutines) processing the same Job
	const numThreads = 10
	results := make(chan struct {
		finding *domain.Finding
		err     error
	}, numThreads)

	for i := 0; i < numThreads; i++ {
		go func(id int) {
			// Each thread gets its own provider with the same client
			p := NewJobProvider(c, cfg)

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mendabot-agent-concurrent",
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "mendabot-watcher",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "remediation.mendabot.io/v1alpha1",
							Kind:               "RemediationJob",
							Name:               "parent",
							UID:                "parent-uid",
							Controller:         ptr(true),
							BlockOwnerDeletion: ptr(true),
						},
					},
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Active: 0,
				},
			}

			finding, err := p.ExtractFinding(job)
			results <- struct {
				finding *domain.Finding
				err     error
			}{finding, err}
		}(i)
	}

	// Collect results
	var findings []*domain.Finding
	var errors []error

	for i := 0; i < numThreads; i++ {
		result := <-results
		if result.err != nil {
			errors = append(errors, result.err)
		}
		if result.finding != nil {
			findings = append(findings, result.finding)
		}
	}

	// Check for errors
	for _, err := range errors {
		t.Errorf("unexpected error: %v", err)
	}

	// All findings should have the same chain depth
	if len(findings) > 0 {
		expectedDepth := findings[0].ChainDepth
		for i, f := range findings {
			if f.ChainDepth != expectedDepth {
				t.Errorf("finding %d: ChainDepth = %d, expected %d (inconsistent reads)",
					i, f.ChainDepth, expectedDepth)
			}
		}

		// Should be 2 (1 from parent + 1)
		if expectedDepth != 2 {
			t.Errorf("ChainDepth = %d, want 2", expectedDepth)
		}

		// All findings should have the same fingerprint
		// (demonstrating they would create duplicate RemediationJobs)
		firstFP, _ := domain.FindingFingerprint(findings[0])
		for i, f := range findings[1:] {
			fp, _ := domain.FindingFingerprint(f)
			if fp != firstFP {
				t.Errorf("finding %d: fingerprint mismatch: %s vs %s", i+1, fp, firstFP)
			}
		}
	}
}
