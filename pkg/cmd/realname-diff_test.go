package cmd

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/diff"
)

// Helper functions for creating test objects

// newConfigMapWithRealname creates a ConfigMap with the realname-diff/realname label
func newConfigMapWithRealname(name, realname string, creationTime time.Time) *unstructured.Unstructured {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.Time{Time: creationTime},
			Labels: map[string]string{
				realNameLabel: realname,
			},
		},
		Data: map[string]string{
			"test": "data",
		},
	}

	unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	return &unstructured.Unstructured{Object: unstructuredObj}
}

// newUnstructuredWithLabels creates an unstructured object with specific labels
func newUnstructuredWithLabels(labels map[string]string) *unstructured.Unstructured {
	labelsInterface := make(map[string]interface{})
	for k, v := range labels {
		labelsInterface[k] = v
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "default",
			},
		},
	}

	if len(labels) > 0 {
		obj.Object["metadata"].(map[string]interface{})["labels"] = labelsInterface
	}

	return obj
}

// newSecretWithRealname creates a Secret with the realname-diff/realname label and last-applied-configuration annotation
func newSecretWithRealname(name, realname string) *unstructured.Unstructured {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				realNameLabel: realname,
			},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"v1","kind":"Secret","data":{"password":"secret"}}`,
			},
		},
		Data: map[string][]byte{
			"password": []byte("secret"),
		},
	}

	unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	return &unstructured.Unstructured{Object: unstructuredObj}
}

// Test functions

// Test_realName tests the realName() function which extracts the realname label from objects
func Test_realName(t *testing.T) {
	tests := []struct {
		name     string
		obj      runtime.Object
		expected string
	}{
		{
			name:     "object with realname label",
			obj:      newUnstructuredWithLabels(map[string]string{realNameLabel: "my-realname"}),
			expected: "my-realname",
		},
		{
			name:     "object without realname label",
			obj:      newUnstructuredWithLabels(map[string]string{"app": "myapp"}),
			expected: "",
		},
		{
			name:     "object with no labels",
			obj:      newUnstructuredWithLabels(map[string]string{}),
			expected: "",
		},
		{
			name:     "object with multiple labels including realname",
			obj:      newUnstructuredWithLabels(map[string]string{"app": "myapp", realNameLabel: "my-realname", "env": "prod"}),
			expected: "my-realname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := realName(tt.obj)
			if result != tt.expected {
				t.Errorf("realName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test_RealnameDiffInfoObject_nameChanged tests the nameChanged() method
func Test_RealnameDiffInfoObject_nameChanged(t *testing.T) {
	tests := []struct {
		name      string
		localName string
		liveObj   runtime.Object
		expected  bool
	}{
		{
			name:      "names are the same",
			localName: "nginx-conf-abc123",
			liveObj:   newConfigMapWithRealname("nginx-conf-abc123", "nginx-conf", time.Now()),
			expected:  false,
		},
		{
			name:      "names are different",
			localName: "nginx-conf-def456",
			liveObj:   newConfigMapWithRealname("nginx-conf-abc123", "nginx-conf", time.Now()),
			expected:  true,
		},
		{
			name:      "no live object",
			localName: "nginx-conf-abc123",
			liveObj:   nil,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localObj := newConfigMapWithRealname(tt.localName, "nginx-conf", time.Now())

			info := &resource.Info{
				Object: tt.liveObj,
			}

			obj := RealnameDiffInfoObject{
				infoObj: diff.InfoObject{
					LocalObj: localObj,
					Info:     info,
				},
			}

			result := obj.nameChanged()
			if result != tt.expected {
				t.Errorf("nameChanged() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test_RealnameDiffInfoObject_Live tests the Live() method - SECURITY CRITICAL
// This test verifies that last-applied-configuration annotation is stripped when names differ
func Test_RealnameDiffInfoObject_Live(t *testing.T) {
	tests := []struct {
		name                              string
		localName                         string
		liveName                          string
		resourceType                      string // "configmap" or "secret"
		expectLastAppliedConfigAnnotation bool
	}{
		{
			name:                              "names unchanged - annotation preserved in ConfigMap",
			localName:                         "nginx-conf-abc123",
			liveName:                          "nginx-conf-abc123",
			resourceType:                      "configmap",
			expectLastAppliedConfigAnnotation: true,
		},
		{
			name:                              "names changed - annotation stripped from ConfigMap (SECURITY)",
			localName:                         "nginx-conf-def456",
			liveName:                          "nginx-conf-abc123",
			resourceType:                      "configmap",
			expectLastAppliedConfigAnnotation: false,
		},
		{
			name:                              "names changed - annotation stripped from Secret (SECURITY CRITICAL)",
			localName:                         "my-secret-def456",
			liveName:                          "my-secret-abc123",
			resourceType:                      "secret",
			expectLastAppliedConfigAnnotation: false,
		},
		{
			name:                              "names unchanged - annotation preserved in Secret",
			localName:                         "my-secret-abc123",
			liveName:                          "my-secret-abc123",
			resourceType:                      "secret",
			expectLastAppliedConfigAnnotation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var liveObj *unstructured.Unstructured
			if tt.resourceType == "secret" {
				liveObj = newSecretWithRealname(tt.liveName, "test-realname")
			} else {
				liveObj = newConfigMapWithRealname(tt.liveName, "test-realname", time.Now())
				// Add the last-applied-configuration annotation to ConfigMap
				annotations := liveObj.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"apiVersion":"v1","kind":"ConfigMap"}`
				liveObj.SetAnnotations(annotations)
			}

			localObj := newConfigMapWithRealname(tt.localName, "test-realname", time.Now())

			obj := RealnameDiffInfoObject{
				infoObj: diff.InfoObject{
					LocalObj: localObj,
					Info: &resource.Info{
						Object: liveObj,
					},
				},
			}

			result := obj.Live()
			if result == nil {
				t.Fatal("Live() returned nil, expected an object")
			}

			// Check if the annotation is present or stripped
			resultUnstructured := result.(*unstructured.Unstructured)
			annotations := resultUnstructured.GetAnnotations()
			_, hasAnnotation := annotations["kubectl.kubernetes.io/last-applied-configuration"]

			if tt.expectLastAppliedConfigAnnotation && !hasAnnotation {
				t.Errorf("Expected last-applied-configuration annotation to be present, but it was missing")
			}
			if !tt.expectLastAppliedConfigAnnotation && hasAnnotation {
				t.Errorf("Expected last-applied-configuration annotation to be stripped for security, but it was present")
			}
		})
	}
}
