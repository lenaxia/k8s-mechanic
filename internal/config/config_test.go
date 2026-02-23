package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/lenaxia/k8s-mendabot/internal/config"
)

func TestFromEnv_AllFieldsPresent(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("SINK_TYPE", "gitlab")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MAX_CONCURRENT_JOBS", "5")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitOpsRepo != "org/repo" {
		t.Errorf("GitOpsRepo: got %q, want %q", cfg.GitOpsRepo, "org/repo")
	}
	if cfg.GitOpsManifestRoot != "kubernetes/" {
		t.Errorf("GitOpsManifestRoot: got %q, want %q", cfg.GitOpsManifestRoot, "kubernetes/")
	}
	if cfg.AgentImage != "ghcr.io/lenaxia/mendabot-agent:latest" {
		t.Errorf("AgentImage: got %q, want %q", cfg.AgentImage, "ghcr.io/lenaxia/mendabot-agent:latest")
	}
	if cfg.AgentNamespace != "mendabot" {
		t.Errorf("AgentNamespace: got %q, want %q", cfg.AgentNamespace, "mendabot")
	}
	if cfg.AgentSA != "mendabot-agent" {
		t.Errorf("AgentSA: got %q, want %q", cfg.AgentSA, "mendabot-agent")
	}
	if cfg.SinkType != "gitlab" {
		t.Errorf("SinkType: got %q, want %q", cfg.SinkType, "gitlab")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.MaxConcurrentJobs != 5 {
		t.Errorf("MaxConcurrentJobs: got %d, want %d", cfg.MaxConcurrentJobs, 5)
	}
}

func TestFromEnv_Defaults(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	os.Unsetenv("SINK_TYPE")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("MAX_CONCURRENT_JOBS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SinkType != "github" {
		t.Errorf("SinkType default: got %q, want %q", cfg.SinkType, "github")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default: got %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.MaxConcurrentJobs != 3 {
		t.Errorf("MaxConcurrentJobs default: got %d, want %d", cfg.MaxConcurrentJobs, 3)
	}
}

func TestFromEnv_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		unsetFn func()
	}{
		{
			name: "missing GITOPS_REPO",
			unsetFn: func() {
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("GITOPS_REPO")
			},
		},
		{
			name: "missing GITOPS_MANIFEST_ROOT",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("GITOPS_MANIFEST_ROOT")
			},
		},
		{
			name: "missing AGENT_IMAGE",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("AGENT_IMAGE")
			},
		},
		{
			name: "missing AGENT_NAMESPACE",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_SA", "mendabot-agent")
				os.Unsetenv("AGENT_NAMESPACE")
			},
		},
		{
			name: "missing AGENT_SA",
			unsetFn: func() {
				t.Setenv("GITOPS_REPO", "org/repo")
				t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
				t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
				t.Setenv("AGENT_NAMESPACE", "mendabot")
				os.Unsetenv("AGENT_SA")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.unsetFn()
			_, err := config.FromEnv()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestFromEnv_InvalidMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid MAX_CONCURRENT_JOBS, got nil")
	}
}

func TestFromEnv_ZeroMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "0")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for MAX_CONCURRENT_JOBS=0, got nil")
	}
}

func TestFromEnv_NegativeMaxConcurrentJobs(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("MAX_CONCURRENT_JOBS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for MAX_CONCURRENT_JOBS=-1, got nil")
	}
}

func TestFromEnv_RemediationJobTTLDefault(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	os.Unsetenv("REMEDIATION_JOB_TTL_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RemediationJobTTLSeconds != 604800 {
		t.Errorf("RemediationJobTTLSeconds default: got %d, want 604800", cfg.RemediationJobTTLSeconds)
	}
}

func TestFromEnv_RemediationJobTTLExplicit(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "86400")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RemediationJobTTLSeconds != 86400 {
		t.Errorf("RemediationJobTTLSeconds: got %d, want 86400", cfg.RemediationJobTTLSeconds)
	}
}

func TestFromEnv_InvalidRemediationJobTTL(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "not-a-number")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid REMEDIATION_JOB_TTL_SECONDS, got nil")
	}
}

func TestFromEnv_ZeroRemediationJobTTL(t *testing.T) {
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
	t.Setenv("REMEDIATION_JOB_TTL_SECONDS", "0")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for REMEDIATION_JOB_TTL_SECONDS=0, got nil")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITOPS_REPO", "org/repo")
	t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
	t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mendabot-agent:latest")
	t.Setenv("AGENT_NAMESPACE", "mendabot")
	t.Setenv("AGENT_SA", "mendabot-agent")
}

func TestFromEnv_StabilisationWindowDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("STABILISATION_WINDOW_SECONDS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 120 * time.Second
	if cfg.StabilisationWindow != want {
		t.Errorf("StabilisationWindow default: got %v, want %v", cfg.StabilisationWindow, want)
	}
}

func TestFromEnv_StabilisationWindowZero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "0")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StabilisationWindow != 0 {
		t.Errorf("StabilisationWindow zero: got %v, want 0", cfg.StabilisationWindow)
	}
}

func TestFromEnv_StabilisationWindowCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "300")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 300 * time.Second
	if cfg.StabilisationWindow != want {
		t.Errorf("StabilisationWindow custom: got %v, want %v", cfg.StabilisationWindow, want)
	}
}

func TestFromEnv_StabilisationWindowNegative(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for STABILISATION_WINDOW_SECONDS=-1, got nil")
	}
}

func TestFromEnv_StabilisationWindowInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STABILISATION_WINDOW_SECONDS", "abc")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for STABILISATION_WINDOW_SECONDS=abc, got nil")
	}
}

func TestFromEnv_SelfRemediationDefaults(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("SELF_REMEDIATION_MAX_DEPTH")
	os.Unsetenv("MENDABOT_UPSTREAM_REPO")
	os.Unsetenv("MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationMaxDepth != 2 {
		t.Errorf("SelfRemediationMaxDepth default: got %d, want 2", cfg.SelfRemediationMaxDepth)
	}
	if cfg.MendabotUpstreamRepo != "lenaxia/k8s-mendabot" {
		t.Errorf("MendabotUpstreamRepo default: got %q, want %q", cfg.MendabotUpstreamRepo, "lenaxia/k8s-mendabot")
	}
	if cfg.DisableUpstreamContributions != false {
		t.Errorf("DisableUpstreamContributions default: got %v, want false", cfg.DisableUpstreamContributions)
	}
}

func TestFromEnv_SelfRemediationCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "3")
	t.Setenv("MENDABOT_UPSTREAM_REPO", "myorg/my-mendabot")
	t.Setenv("MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS", "true")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SelfRemediationMaxDepth != 3 {
		t.Errorf("SelfRemediationMaxDepth custom: got %d, want 3", cfg.SelfRemediationMaxDepth)
	}
	if cfg.MendabotUpstreamRepo != "myorg/my-mendabot" {
		t.Errorf("MendabotUpstreamRepo custom: got %q, want %q", cfg.MendabotUpstreamRepo, "myorg/my-mendabot")
	}
	if cfg.DisableUpstreamContributions != true {
		t.Errorf("DisableUpstreamContributions custom: got %v, want true", cfg.DisableUpstreamContributions)
	}
}

func TestFromEnv_SelfRemediationInvalidDepth(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "abc")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_SelfRemediationNegativeDepth(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SELF_REMEDIATION_MAX_DEPTH", "-1")

	_, err := config.FromEnv()
	if err == nil {
		t.Fatal("expected error for negative SELF_REMEDIATION_MAX_DEPTH, got nil")
	}
}

func TestFromEnv_DisableUpstreamContributionsVariants(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			setRequiredEnv(t)
			if tt.envValue != "" {
				t.Setenv("MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS", tt.envValue)
			} else {
				os.Unsetenv("MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS")
			}

			cfg, err := config.FromEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.DisableUpstreamContributions != tt.expected {
				t.Errorf("DisableUpstreamContributions for %q: got %v, want %v", tt.envValue, cfg.DisableUpstreamContributions, tt.expected)
			}
		})
	}
}
