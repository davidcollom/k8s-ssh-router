package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davidcollom/k8s-ssh-router/pkg/sshserver"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	reconcileInterval int
	sshPort           int
	metricsPort       int
	namespace         string
	privateKeyPath    string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "k8s-ssh-router",
		Short: "Kubernetes SSH router",
		Run: func(cmd *cobra.Command, args []string) {
			RunServer()
		},
	}

	rootCmd.Flags().IntVar(&reconcileInterval, "reconcile-interval", 30, "Reconcile interval in seconds")
	rootCmd.Flags().IntVar(&sshPort, "ssh-port", 2222, "SSH server port")
	rootCmd.Flags().IntVar(&metricsPort, "metrics-port", 9090, "Metrics server port")
	rootCmd.Flags().StringVar(&namespace, "namespace", "", "Kubernetes namespace")
	rootCmd.Flags().StringVar(&privateKeyPath, "private-key", "/etc/ssh/ssh_host_rsa_key", "Path to private key")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Error executing root command: %v", err)
	}
}

func RunServer() {
	// Cancelling the context on SIGINT or SIGTERM lets the server shut the
	// listeners down gracefully instead of being killed mid request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			log.Fatalf("Failed to create Kubernetes client config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	sshserver.RunServer(ctx, reconcileInterval, sshPort, metricsPort, namespace, privateKeyPath, clientset, k8sConfig)
}
