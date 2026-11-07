package sshserver

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// listerAt adapts a fully materialised slice of directory entries to the
// sftp.ListerAt interface.
type listerAt struct {
	entries []os.FileInfo
}

// ListAt copies the entries starting at offset into f and reports how many
// were written. It returns io.EOF once the entries are exhausted.
func (l *listerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("negative offset %d", offset)
	}
	if offset >= int64(len(l.entries)) {
		return 0, io.EOF
	}
	n := copy(f, l.entries[offset:])
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
}

// lsFileInfo is the os.FileInfo implementation for an entry parsed out of an
// "ls -l" listing.
type lsFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f lsFileInfo) Name() string       { return f.name }
func (f lsFileInfo) Size() int64        { return f.size }
func (f lsFileInfo) Mode() os.FileMode  { return f.mode }
func (f lsFileInfo) ModTime() time.Time { return time.Time{} }
func (f lsFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f lsFileInfo) Sys() any           { return nil }

// parseLsLong parses the output of "ls -l" into directory entries.
//
// The listing has the shape:
//
//	-rw-r--r-- 1 root root  123 Nov  2 09:32 file.txt
//	drwxr-xr-x 2 root root 4096 Nov  2 09:32 subdir
//	lrwxrwxrwx 1 root root   11 Nov  2 09:32 link -> target
//
// The date is always three whitespace separated fields, so the name is
// everything that follows it.
func parseLsLong(out []byte) ([]os.FileInfo, error) {
	var entries []os.FileInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		// mode, links, owner, group, size, month, day, time-or-year, name...
		if len(fields) < 9 {
			// "total 12" and similar headers are not entries.
			continue
		}
		if len(fields[0]) != 10 {
			continue
		}
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse size %q: %v", fields[4], err)
		}
		name := strings.Join(fields[8:], " ")
		if idx := strings.Index(name, " -> "); idx >= 0 {
			// Symbolic links are listed as "link -> target".
			name = name[:idx]
		}
		entries = append(entries, lsFileInfo{
			name: name,
			size: size,
			mode: parseLsMode(fields[0]),
		})
	}
	return entries, nil
}

// parseLsMode converts the ten character mode string of an "ls -l" listing
// into an os.FileMode.
func parseLsMode(mode string) os.FileMode {
	if len(mode) != 10 {
		return 0
	}
	var m os.FileMode
	switch mode[0] {
	case 'd':
		m |= os.ModeDir
	case 'l':
		m |= os.ModeSymlink
	case 'p':
		m |= os.ModeNamedPipe
	case 's':
		m |= os.ModeSocket
	case 'b':
		m |= os.ModeDevice
	case 'c':
		m |= os.ModeDevice | os.ModeCharDevice
	}

	perms := []struct {
		char  byte
		owner os.FileMode
	}{
		{'r', 0400}, {'w', 0200}, {'x', 0100},
		{'r', 0040}, {'w', 0020}, {'x', 0010},
		{'r', 0004}, {'w', 0002}, {'x', 0001},
	}
	for i, p := range perms {
		if mode[i+1] == p.char {
			m |= p.owner
		}
	}
	return m
}

// shellQuote quotes a path so that it can safely be embedded in the shell
// command that is executed inside the pod.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
