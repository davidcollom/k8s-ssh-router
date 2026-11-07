package k8s

import (
	"context"
	"fmt"
	"log"

	"github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getClientset() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			log.Fatalf("failed to create Kubernetes client config: %v", err)
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

// WatchSecretsClusterWide watches the secrets that are labelled for SSH
// access and keeps the local credential cache up to date. It stops watching
// once the given context is cancelled.
func WatchSecretsClusterWide(ctx context.Context, reconcileInterval int, namespace string, readyCh chan struct{}) (watch.Interface, error) {
	clientset, err := getClientset()
	if err != nil {
		return nil, err
	}

	go startReconciliation(ctx, clientset, reconcileInterval, namespace)

	watcher, err := clientset.CoreV1().Secrets(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: "ssh=user",
	})
	if err != nil {
		return nil, err
	}

	log.Println("Starting secret watcher...")
	readyCh <- struct{}{}
	go processSecretEvents(ctx, watcher.ResultChan())

	return watcher, nil
}

// processSecretEvents applies the secret watch events to the local cache
// until the context is cancelled or the watch channel is closed.
func processSecretEvents(ctx context.Context, ch <-chan watch.Event) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping secret watcher...")
			return
		case event, ok := <-ch:
			if !ok {
				log.Println("Secret watcher channel closed")
				return
			}
			handleSecretEvent(event)
		}
	}
}

// handleSecretEvent applies a single secret watch event to the local cache.
func handleSecretEvent(event watch.Event) {
	switch event.Type {
	case watch.Added, watch.Modified:
		log.Printf("Processing %s event", event.Type)
		secret, ok := event.Object.(*corev1.Secret)
		if !ok {
			log.Printf("Unexpected object type %T for %s event", event.Object, event.Type)
			return
		}
		processSecret(secret)
	case watch.Deleted:
		log.Println("Processing deleted event")
		secret, ok := event.Object.(*corev1.Secret)
		if !ok {
			log.Printf("Unexpected object type %T for %s event", event.Object, event.Type)
			return
		}
		namespace := secret.Namespace
		username := string(secret.Data["username"])
		usernameWithNamespace := fmt.Sprintf("%s-%s", namespace, username)
		localCache.Delete(usernameWithNamespace)
		log.Printf("Deleted secret: %s, cache size: %d\n", usernameWithNamespace, localCache.ItemCount())
	}
}

func processSecret(secret *corev1.Secret) {
	namespace := secret.Namespace
	username := string(secret.Data["username"])
	usernameWithNamespace := fmt.Sprintf("%s-%s", namespace, username)
	secretData := map[string]string{
		"password":         string(secret.Data["password"]),
		"publicKey":        string(secret.Data["publicKey"]),
		"service":          string(secret.Data["service"]),
		"podLabelSelector": string(secret.Data["podLabelSelector"]),
		"containerName":    string(secret.Data["containerName"]),
		"shell":            string(secret.Data["shell"]),
	}
	localCache.Set(usernameWithNamespace, secretData, cache.DefaultExpiration)
	log.Printf("Added/Modified secret: %s, cache size: %d\n", usernameWithNamespace, localCache.ItemCount())
}
