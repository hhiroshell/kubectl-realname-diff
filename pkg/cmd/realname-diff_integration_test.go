//go:build integration
// +build integration

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg       *rest.Config
	testEnv   *envtest.Environment
	k8sClient client.Client
)

// TestMain sets up the envtest environment for integration tests
func TestMain(m *testing.M) {
	// Get binary assets directory from environment or use default
	binDir := os.Getenv("KUBEBUILDER_ASSETS")
	if binDir == "" {
		// Fallback to absolute path based on current working directory
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
			os.Exit(1)
		}
		// From pkg/cmd, go up two levels to reach project root
		binDir = filepath.Join(cwd, "..", "..", "testbin", "k8s", "1.29.0-linux-amd64")
		binDir = filepath.Clean(binDir)
	}

	// Setup envtest environment
	testEnv = &envtest.Environment{
		BinaryAssetsDirectory:    binDir,
		ControlPlaneStartTimeout: 60 * time.Second,
		ControlPlaneStopTimeout:  60 * time.Second,
		AttachControlPlaneOutput: true, // For debugging
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start test environment: %v\n", err)
		os.Exit(1)
	}

	// Create controller-runtime client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
		testEnv.Stop()
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown
	if err := testEnv.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stop test environment: %v\n", err)
		os.Exit(1)
	}

	os.Exit(code)
}

// setupTestNamespace creates a new namespace for each test and ensures cleanup
func setupTestNamespace(t *testing.T) string {
	t.Helper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-realname-diff-",
		},
	}

	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("failed to create test namespace: %v", err)
	}

	// Cleanup after test
	t.Cleanup(func() {
		ctx := context.Background()
		if err := k8sClient.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
			t.Logf("warning: failed to delete test namespace %s: %v", ns.Name, err)
		}
	})

	return ns.Name
}

// createConfigMapWithRealname creates a ConfigMap in the API server with realname label
// Note: creationTime parameter is ignored as CreationTimestamp is set by the API server
func createConfigMapWithRealname(t *testing.T, namespace, name, realname string, creationTime time.Time) *unstructured.Unstructured {
	t.Helper()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				realNameLabel: realname,
			},
		},
		Data: map[string]string{
			"test": "data",
		},
	}

	if err := k8sClient.Create(context.Background(), cm); err != nil {
		t.Fatalf("failed to create ConfigMap: %v", err)
	}

	// Convert to unstructured for consistency with production code
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	if err != nil {
		t.Fatalf("failed to convert to unstructured: %v", err)
	}

	return &unstructured.Unstructured{Object: unstructuredObj}
}

// createSecretWithRealname creates a Secret in the API server with realname label
func createSecretWithRealname(t *testing.T, namespace, name, realname string) *unstructured.Unstructured {
	t.Helper()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				realNameLabel: realname,
			},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"v1","kind":"Secret"}`,
			},
		},
		Data: map[string][]byte{
			"password": []byte("secret"),
		},
	}

	if err := k8sClient.Create(context.Background(), secret); err != nil {
		t.Fatalf("failed to create Secret: %v", err)
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	if err != nil {
		t.Fatalf("failed to convert to unstructured: %v", err)
	}

	return &unstructured.Unstructured{Object: unstructuredObj}
}

// createResourceInfo creates a resource.Info object for testing getWithRealName
func createResourceInfo(t *testing.T, namespace string, gvk schema.GroupVersionKind) *resource.Info {
	t.Helper()

	// Get REST mapper from k8sClient
	mapper := k8sClient.RESTMapper()

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		t.Fatalf("failed to get REST mapping: %v", err)
	}

	// Create a REST client configured for unstructured objects
	// Use UnstructuredJSONScheme to ensure responses are decoded as unstructured
	restConfig := rest.CopyConfig(cfg)
	gv := mapping.GroupVersionKind.GroupVersion()
	restConfig.GroupVersion = &gv
	restConfig.APIPath = "/api"
	if mapping.GroupVersionKind.Group != "" {
		restConfig.APIPath = "/apis"
	}
	restConfig.NegotiatedSerializer = resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer

	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		t.Fatalf("failed to create REST client: %v", err)
	}

	return &resource.Info{
		Namespace: namespace,
		Mapping:   mapping,
		Client:    restClient,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": gvk.GroupVersion().String(),
				"kind":       gvk.Kind,
			},
		},
	}
}

// assertResourceMatches verifies the retrieved resource matches expected values
func assertResourceMatches(t *testing.T, info *resource.Info, expectedName, expectedRealname string) {
	t.Helper()

	if info.Object == nil {
		t.Fatal("expected object to be set, got nil")
	}

	obj, ok := info.Object.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("expected *unstructured.Unstructured, got %T", info.Object)
	}

	if obj.GetName() != expectedName {
		t.Errorf("expected name %q, got %q", expectedName, obj.GetName())
	}

	labels := obj.GetLabels()
	if realname, ok := labels[realNameLabel]; !ok || realname != expectedRealname {
		t.Errorf("expected realname label %q, got %q", expectedRealname, realname)
	}
}

// assertResourceVersionCaptured verifies resource version is captured
func assertResourceVersionCaptured(t *testing.T, info *resource.Info) {
	t.Helper()

	if info.ResourceVersion == "" {
		t.Error("expected resource version to be captured, got empty string")
	}
}

// TestGetWithRealName_SingleMatch tests getWithRealName with a single matching ConfigMap
func TestGetWithRealName_SingleMatch(t *testing.T) {
	namespace := setupTestNamespace(t)

	// Create ConfigMap in API server
	createConfigMapWithRealname(t, namespace, "my-config-abc123", "my-config", time.Time{})

	// Create Info and call getWithRealName
	info := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := getWithRealName(info, "my-config", targetSelectionStrategyError)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResourceMatches(t, info, "my-config-abc123", "my-config")
	assertResourceVersionCaptured(t, info)
}

// TestGetWithRealName_MultipleMatches tests getWithRealName with multiple matching resources
func TestGetWithRealName_MultipleMatches(t *testing.T) {
	tests := []struct {
		name         string
		strategy     string
		expectError  bool
		expectedName string
	}{
		{
			name:        "error strategy returns error with multiple matches",
			strategy:    targetSelectionStrategyError,
			expectError: true,
		},
		{
			name:         "latest strategy selects newest resource",
			strategy:     targetSelectionStrategyLatest,
			expectError:  false,
			expectedName: "my-config-ghi789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := setupTestNamespace(t)

			// Create multiple resources with SAME realname sequentially
			// API server assigns CreationTimestamp, so we create oldest to newest with delays
			createConfigMapWithRealname(t, namespace, "my-config-abc123", "my-config", time.Time{})
			// Ensure distinct CreationTimestamps. K8s API server records timestamps with 1-second granularity, so sub-second intervals may result in identical timestamps.
			time.Sleep(1 * time.Second)
			createConfigMapWithRealname(t, namespace, "my-config-def456", "my-config", time.Time{})
			time.Sleep(1 * time.Second)
			createConfigMapWithRealname(t, namespace, "my-config-ghi789", "my-config", time.Time{})
			time.Sleep(10 * time.Millisecond) // Allow final resource to be fully persisted

			info := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
			err := getWithRealName(info, "my-config", tt.strategy)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != "multiple objects have same realname label: realname=my-config" {
					t.Errorf("unexpected error message: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assertResourceMatches(t, info, tt.expectedName, "my-config")
			}
		})
	}
}

// TestGetWithRealName_Fallback tests fallback to Get() when no realname label matches
func TestGetWithRealName_Fallback(t *testing.T) {
	namespace := setupTestNamespace(t)

	// Create ConfigMap WITHOUT realname label
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"test": "data",
		},
	}
	if err := k8sClient.Create(context.Background(), cm); err != nil {
		t.Fatalf("failed to create ConfigMap: %v", err)
	}

	// getWithRealName should fallback to Get() by name
	info := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := getWithRealName(info, "my-config", targetSelectionStrategyError)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Object == nil {
		t.Fatal("expected object to be set, got nil")
	}

	obj := info.Object.(*unstructured.Unstructured)
	if obj.GetName() != "my-config" {
		t.Errorf("expected name %q, got %q", "my-config", obj.GetName())
	}
}

// TestGetWithRealName_NotFound tests behavior when resource is not found
func TestGetWithRealName_NotFound(t *testing.T) {
	namespace := setupTestNamespace(t)

	// Don't create any resources
	info := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := getWithRealName(info, "nonexistent", targetSelectionStrategyError)

	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got: %v", err)
	}
}

// TestGetWithRealName_NamespaceIsolation tests that resources in different namespaces don't interfere
func TestGetWithRealName_NamespaceIsolation(t *testing.T) {
	ns1 := setupTestNamespace(t)
	ns2 := setupTestNamespace(t)

	// Create resource in ns1
	createConfigMapWithRealname(t, ns1, "my-config-abc", "my-config", time.Time{})

	// Search in ns2 should not find it
	info := createResourceInfo(t, ns2, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := getWithRealName(info, "my-config", targetSelectionStrategyError)

	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound (namespace isolation), got: %v", err)
	}
}

// TestGetWithRealName_DeepCopy tests that returned object is deep copied
func TestGetWithRealName_DeepCopy(t *testing.T) {
	namespace := setupTestNamespace(t)

	createConfigMapWithRealname(t, namespace, "my-config-abc", "my-config", time.Time{})

	// First retrieval
	info := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := getWithRealName(info, "my-config", targetSelectionStrategyError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify object is set
	if info.Object == nil {
		t.Fatal("expected object to be set, got nil")
	}

	// Modify the object
	obj := info.Object.(*unstructured.Unstructured)
	originalName := obj.GetName()
	obj.SetLabels(map[string]string{"modified": "true"})

	// Second retrieval should get unmodified object
	info2 := createResourceInfo(t, namespace, corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err = getWithRealName(info2, "my-config", targetSelectionStrategyError)
	if err != nil {
		t.Fatalf("unexpected error on re-fetch: %v", err)
	}

	obj2 := info2.Object.(*unstructured.Unstructured)
	if obj2.GetName() != originalName {
		t.Errorf("expected name %q, got %q", originalName, obj2.GetName())
	}

	// Verify the original label is still there (not modified label)
	labels := obj2.GetLabels()
	if _, ok := labels["modified"]; ok {
		t.Error("deep copy failed: modification affected re-fetched object")
	}
	if labels[realNameLabel] != "my-config" {
		t.Error("expected realname label to be preserved")
	}
}
