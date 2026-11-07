package sshserver

import (
	"io"
	"log"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// handleSFTP handles SFTP requests.
func handleSFTP(channel ssh.Channel, clientset kubernetes.Interface, config *rest.Config, namespace, podName, containerName string) {
	handler := &SFTPHandler{
		Clientset:     clientset,
		Config:        config,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,
	}

	// pkg/sftp v1.11 dropped the per-operation setters that used to hang off
	// *sftp.Server (SetFileGet, SetFilePut, ...) and replaced them with a
	// Handlers value that is handed to NewRequestServer.
	server := sftp.NewRequestServer(channel, sftp.Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	})

	if err := server.Serve(); err == io.EOF {
		server.Close()
		log.Printf("SFTP client exited session.")
	} else if err != nil {
		log.Printf("SFTP server completed with error: %v", err)
		channel.Close()
	}
}
