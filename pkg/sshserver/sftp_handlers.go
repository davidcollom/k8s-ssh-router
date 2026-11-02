package sshserver

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pkg/sftp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Compile-time checks that SFTPHandler satisfies the handler interfaces that
// pkg/sftp v1.11+ expects in a sftp.Handlers value.
var (
	_ sftp.FileReader = (*SFTPHandler)(nil)
	_ sftp.FileWriter = (*SFTPHandler)(nil)
	_ sftp.FileCmder  = (*SFTPHandler)(nil)
	_ sftp.FileLister = (*SFTPHandler)(nil)
)

type SFTPHandler struct {
	Clientset     kubernetes.Interface
	Config        *rest.Config
	Namespace     string
	PodName       string
	ContainerName string
}

// Fileread serves an SFTP "get" request by cat-ing the file out of the pod.
func (h *SFTPHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	cmd := fmt.Sprintf("cat %s", shellQuote(r.Filepath))
	stdout, stderr, err := h.execInPod(cmd)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v, stderr: %s", err, stderr)
	}
	return bytes.NewReader(stdout), nil
}

// Filewrite serves an SFTP "put" request. The returned writer buffers the
// upload and flushes it into the pod when it is closed.
func (h *SFTPHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	cmd := fmt.Sprintf("cat > %s", shellQuote(r.Filepath))
	stdin := new(bytes.Buffer)
	writer := &sftpFileWriter{stdin: stdin, handler: h, cmd: cmd}
	return writer, nil
}

// Filecmd serves the mutating SFTP requests (remove, rename, chmod). pkg/sftp
// v1.11+ routes every one of them through a single Filecmd callback and tells
// us which one it is through Request.Method.
func (h *SFTPHandler) Filecmd(r *sftp.Request) error {
	var cmd string
	switch r.Method {
	case "Remove":
		cmd = fmt.Sprintf("rm %s", shellQuote(r.Filepath))
	case "Rename", "PosixRename":
		cmd = fmt.Sprintf("mv %s %s", shellQuote(r.Filepath), shellQuote(r.Target))
	case "Chmod":
		attrs := r.Attributes()
		if attrs == nil {
			return fmt.Errorf("chmod request for %s carried no attributes", r.Filepath)
		}
		cmd = fmt.Sprintf("chmod %o %s", os.FileMode(attrs.Mode).Perm(), shellQuote(r.Filepath))
	default:
		return fmt.Errorf("unsupported sftp command %q", r.Method)
	}

	_, stderr, err := h.execInPod(cmd)
	if err != nil {
		return fmt.Errorf("error running %q: %v, stderr: %s", r.Method, err, stderr)
	}
	return nil
}

// Filelist serves an SFTP "list" request by listing the directory inside the
// pod.
func (h *SFTPHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	cmd := fmt.Sprintf("ls -l %s", shellQuote(r.Filepath))
	stdout, stderr, err := h.execInPod(cmd)
	if err != nil {
		return nil, fmt.Errorf("error listing directory: %v, stderr: %s", err, stderr)
	}
	entries, err := parseLsLong(stdout)
	if err != nil {
		return nil, fmt.Errorf("error parsing directory listing: %v", err)
	}
	return &listerAt{entries: entries}, nil
}

func (h *SFTPHandler) execInPod(cmd string) ([]byte, []byte, error) {
	req := h.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(h.PodName).
		Namespace(h.Namespace).
		SubResource("exec").
		Param("container", h.ContainerName).
		Param("command", "/bin/sh").
		Param("command", "-c").
		Param("command", cmd).
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false")

	exec, err := remotecommand.NewSPDYExecutor(h.Config, "POST", req.URL())
	if err != nil {
		return nil, nil, err
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return nil, nil, err
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}

type sftpFileWriter struct {
	stdin   *bytes.Buffer
	handler *SFTPHandler
	cmd     string
}

func (w *sftpFileWriter) WriteAt(p []byte, off int64) (int, error) {
	_, err := w.stdin.Write(p)
	return len(p), err
}

func (w *sftpFileWriter) Close() error {
	_, stderr, err := w.handler.execInPod(w.cmd)
	if err != nil {
		return fmt.Errorf("error writing file: %v, stderr: %s", err, stderr)
	}
	return nil
}
