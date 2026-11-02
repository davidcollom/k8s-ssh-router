package sshserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/davidcollom/k8s-ssh-router/pkg/auth"
	"github.com/davidcollom/k8s-ssh-router/pkg/k8s"
	"github.com/davidcollom/k8s-ssh-router/pkg/metrics"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// RunServer starts the secret watcher, the metrics endpoint and the SSH
// server. It blocks until the SSH server stops, which happens when the given
// context is cancelled.
func RunServer(ctx context.Context, reconcileInterval, sshPort, metricsPort int, namespace, privateKeyPath string, clientset kubernetes.Interface, config *rest.Config) {
	go watchSecrets(ctx, reconcileInterval, namespace)
	go serveMetrics(ctx, metricsPort)
	serveSSH(ctx, sshPort, privateKeyPath, clientset, config)
}

// watchSecrets keeps the credential cache in sync with the secrets that are
// labelled for SSH access.
func watchSecrets(ctx context.Context, reconcileInterval int, namespace string) {
	readyCh := make(chan struct{})
	if _, err := k8s.WatchSecretsClusterWide(ctx, reconcileInterval, namespace, readyCh); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}
}

// serveMetrics runs the Prometheus metrics endpoint until the context is
// cancelled.
func serveMetrics(ctx context.Context, port int) {
	if err := metrics.StartMetricsServer(ctx, port); err != nil {
		log.Printf("Metrics server stopped: %v", err)
	}
}

// serveSSH accepts SSH connections until the context is cancelled.
func serveSSH(ctx context.Context, port int, privateKeyPath string, clientset kubernetes.Interface, restConfig *rest.Config) {
	sshConfig := &ssh.ServerConfig{
		NoClientAuth:      false,
		PasswordCallback:  auth.PasswordCallback,
		PublicKeyCallback: auth.PublicKeyCallback,
	}

	privateBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Fatalf("Failed to load private key from %s: %v", privateKeyPath, err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatalf("Failed to parse private key: %v", err)
	}

	sshConfig.AddHostKey(private)

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", port, err)
	}
	log.Printf("Listening on 0.0.0.0:%d...", port)

	// Closing the listener makes Accept return so that the server can stop.
	go closeListenerOnCancel(ctx, listener)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("Shutting down SSH server")
				return
			}
			log.Printf("Failed to accept incoming connection: %v", err)
			continue
		}
		go HandleSSHConnection(conn, sshConfig, clientset, restConfig)
	}
}

// closeListenerOnCancel closes the listener once the context is cancelled.
func closeListenerOnCancel(ctx context.Context, listener net.Listener) {
	<-ctx.Done()
	if err := listener.Close(); err != nil {
		log.Printf("Failed to close listener: %v", err)
	}
}
