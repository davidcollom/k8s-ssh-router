package sshserver

import (
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var AppFS afero.Fs // Define the AppFS variable here

// Function to start an SFTP server
func startSFTPServer(t *testing.T, privateKeyPath string, fs afero.Fs, clientset kubernetes.Interface, config *rest.Config) string {
	privateBytes, err := ioutil.ReadFile(privateKeyPath)
	require.NoError(t, err, "Failed to read private key")

	signer, err := ssh.ParsePrivateKey(privateBytes)
	require.NoError(t, err, "Failed to parse private key")

	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to listen on 127.0.0.1:0")

	go func() {
		for {
			conn, err := listener.Accept()
			require.NoError(t, err, "Failed to accept connection")

			_, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
			require.NoError(t, err, "Failed to establish SSH connection")

			go ssh.DiscardRequests(reqs)
			for newChannel := range chans {
				channel, requests, err := newChannel.Accept()
				require.NoError(t, err, "Failed to accept channel")

				go ssh.DiscardRequests(requests)
				go handleSFTP(channel, clientset, config, "default", "test-pod", "test-container")
			}
		}
	}()

	return listener.Addr().String()
}

// Function to create an SFTP client
func createSFTPClient(t *testing.T, addr string) *sftp.Client {
	clientConfig := &ssh.ClientConfig{
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", addr, clientConfig)
	require.NoError(t, err, "Failed to dial %s", addr)

	client, err := sftp.NewClient(conn)
	require.NoError(t, err, "Failed to create SFTP client")

	return client
}

func TestMain(m *testing.M) {
	// Use the in-memory filesystem for testing
	AppFS = afero.NewMemMapFs()
	code := m.Run()
	os.Exit(code)
}
