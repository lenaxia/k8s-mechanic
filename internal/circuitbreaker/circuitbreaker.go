package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ConfigMapName is the name of the ConfigMap used to store circuit breaker state
	ConfigMapName = "mendabot-circuit-breaker"

	// ConfigMapNamespace is the namespace where the ConfigMap is stored
	// This should be the same as the agent namespace
	ConfigMapNamespaceKey = "agent-namespace"

	// LastSelfRemediationKey is the key in the ConfigMap data storing the last self-remediation time
	LastSelfRemediationKey = "last-self-remediation"

	// DefaultCooldown is the default cooldown period between self-remediations
	DefaultCooldown = 5 * time.Minute
)

// CircuitBreaker manages a persistent circuit breaker state using a ConfigMap.
// It provides thread-safe operations and persists state across controller restarts.
type CircuitBreaker struct {
	client    client.Client
	namespace string
	cooldown  time.Duration

	// mutex protects concurrent access to the in-memory cache
	mutex               sync.RWMutex
	lastSelfRemediation time.Time
	initialized         bool
}

// New creates a new CircuitBreaker instance.
func New(client client.Client, namespace string, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		client:    client,
		namespace: namespace,
		cooldown:  cooldown,
	}
}

// ShouldAllow checks if a self-remediation should be allowed based on the cooldown period.
// If allowed, it updates the last self-remediation time in the ConfigMap.
// If cooldown is zero, always returns true (circuit breaker disabled).
func (cb *CircuitBreaker) ShouldAllow(ctx context.Context) (bool, time.Duration, error) {
	// If cooldown is zero, circuit breaker is disabled
	if cb.cooldown == 0 {
		return true, 0, nil
	}

	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Load state if not initialized
	if !cb.initialized {
		if err := cb.loadState(ctx); err != nil {
			return false, 0, fmt.Errorf("failed to load circuit breaker state: %w", err)
		}
	}

	// Check cooldown
	timeSinceLast := time.Since(cb.lastSelfRemediation)
	if timeSinceLast < cb.cooldown {
		remaining := cb.cooldown - timeSinceLast
		return false, remaining, nil
	}

	// Update last self-remediation time
	cb.lastSelfRemediation = time.Now()
	if err := cb.saveState(ctx); err != nil {
		return false, 0, fmt.Errorf("failed to save circuit breaker state: %w", err)
	}

	return true, 0, nil
}

// GetLastSelfRemediation returns the last self-remediation time.
func (cb *CircuitBreaker) GetLastSelfRemediation(ctx context.Context) (time.Time, error) {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if !cb.initialized {
		if err := cb.loadState(ctx); err != nil {
			return time.Time{}, fmt.Errorf("failed to load circuit breaker state: %w", err)
		}
	}

	return cb.lastSelfRemediation, nil
}

// loadState loads the circuit breaker state from the ConfigMap.
func (cb *CircuitBreaker) loadState(ctx context.Context) error {
	logger := log.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	err := cb.client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: cb.namespace,
	}, configMap)

	if err != nil {
		if errors.IsNotFound(err) {
			// ConfigMap doesn't exist yet, use zero time
			cb.lastSelfRemediation = time.Time{}
			cb.initialized = true
			logger.Info("Circuit breaker ConfigMap not found, using default state")
			return nil
		}
		return fmt.Errorf("failed to get circuit breaker ConfigMap: %w", err)
	}

	// Parse last self-remediation time from ConfigMap data
	if timestampStr, exists := configMap.Data[LastSelfRemediationKey]; exists && timestampStr != "" {
		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			logger.Error(err, "Failed to parse last self-remediation time from ConfigMap, using default")
			cb.lastSelfRemediation = time.Time{}
		} else {
			cb.lastSelfRemediation = timestamp
		}
	} else {
		cb.lastSelfRemediation = time.Time{}
	}

	cb.initialized = true
	return nil
}

// saveState saves the circuit breaker state to the ConfigMap.
func (cb *CircuitBreaker) saveState(ctx context.Context) error {
	logger := log.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: cb.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mendabot",
				"app.kubernetes.io/component":  "circuit-breaker",
			},
		},
		Data: map[string]string{
			LastSelfRemediationKey: cb.lastSelfRemediation.Format(time.RFC3339),
			ConfigMapNamespaceKey:  cb.namespace,
		},
	}

	// Try to get existing ConfigMap first to check if it exists
	existingConfigMap := &corev1.ConfigMap{}
	err := cb.client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: cb.namespace,
	}, existingConfigMap)

	if err != nil {
		if errors.IsNotFound(err) {
			// ConfigMap doesn't exist, create it
			err = cb.client.Create(ctx, configMap)
			if err != nil {
				return fmt.Errorf("failed to create circuit breaker ConfigMap: %w", err)
			}
			logger.Info("Created circuit breaker ConfigMap")
			return nil
		}
		// Other error getting ConfigMap
		return fmt.Errorf("failed to get circuit breaker ConfigMap: %w", err)
	}

	// ConfigMap exists, update it
	configMap.SetResourceVersion(existingConfigMap.GetResourceVersion())
	err = cb.client.Update(ctx, configMap)
	if err != nil {
		return fmt.Errorf("failed to update circuit breaker ConfigMap: %w", err)
	}

	return nil
}

// Reset resets the circuit breaker state (for testing or manual intervention).
func (cb *CircuitBreaker) Reset(ctx context.Context) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.lastSelfRemediation = time.Time{}
	cb.initialized = true

	// Also update the ConfigMap
	return cb.saveState(ctx)
}
