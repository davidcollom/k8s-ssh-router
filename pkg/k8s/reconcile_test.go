package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientFake "k8s.io/client-go/kubernetes/fake"
)

func TestReconcileCache(t *testing.T) {
	clientset := clientFake.NewSimpleClientset()

	// Create a secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			Labels: map[string]string{
				"ssh": "user",
			},
		},
		Data: map[string][]byte{
			"username": []byte("testuser"),
			"password": []byte("testpassword"),
		},
	}
	clientset.CoreV1().Secrets("default").Create(context.TODO(), secret, metav1.CreateOptions{})

	// Start reconciliation
	reconcileCache(context.Background(), clientset, "default")

	// Verify secret in cache
	usernameWithNamespace := "default-testuser"
	cachedSecret, found := GetSecretFromCache(usernameWithNamespace)
	assert.True(t, found, "Secret should be found in cache after reconciliation")
	expectedData := map[string]string{
		"password":         "testpassword",
		"publicKey":        "",
		"service":          "",
		"podLabelSelector": "",
		"containerName":    "",
		"shell":            "",
	}
	assert.Equal(t, expectedData, cachedSecret, "Secret data should match")

	// Modify secret and start reconciliation again
	secret.Data["password"] = []byte("newpassword")
	clientset.CoreV1().Secrets("default").Update(context.TODO(), secret, metav1.UpdateOptions{})

	reconcileCache(context.Background(), clientset, "default")
	cachedSecret, found = GetSecretFromCache(usernameWithNamespace)
	assert.True(t, found, "Secret should be found in cache after modification")
	expectedData["password"] = "newpassword"
	assert.Equal(t, expectedData, cachedSecret, "Modified secret data should match")

	// Delete secret and start reconciliation again
	clientset.CoreV1().Secrets("default").Delete(context.TODO(), secret.Name, metav1.DeleteOptions{})

	reconcileCache(context.Background(), clientset, "default")
	_, found = GetSecretFromCache(usernameWithNamespace)
	assert.False(t, found, "Secret should not be found in cache after deletion")
}
