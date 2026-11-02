package sshserver

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLsLong(t *testing.T) {
	listing := `total 12
-rw-r--r-- 1 root root    123 Nov  2 09:32 file.txt
drwxr-xr-x 2 root root   4096 Nov  2 09:32 subdir
lrwxrwxrwx 1 root root     11 Nov  2 09:32 link -> target
-rw-rw---- 1 root root    999 Nov 22  2024 older.txt
`

	entries, err := parseLsLong([]byte(listing))
	require.NoError(t, err)
	require.Len(t, entries, 4)

	require.Equal(t, "file.txt", entries[0].Name())
	require.Equal(t, int64(123), entries[0].Size())
	require.Equal(t, os.FileMode(0644), entries[0].Mode().Perm())
	require.False(t, entries[0].IsDir())

	require.Equal(t, "subdir", entries[1].Name())
	require.Equal(t, int64(4096), entries[1].Size())
	require.True(t, entries[1].IsDir())
	require.Equal(t, os.ModeDir, entries[1].Mode()&os.ModeDir)

	require.Equal(t, "link", entries[2].Name())
	require.True(t, entries[2].Mode()&os.ModeSymlink != 0)

	require.Equal(t, "older.txt", entries[3].Name())
	require.Equal(t, int64(999), entries[3].Size())
	require.Equal(t, os.FileMode(0660), entries[3].Mode().Perm())
}

func TestParseLsLongEmpty(t *testing.T) {
	entries, err := parseLsLong([]byte(""))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestListerAt(t *testing.T) {
	entries := []os.FileInfo{
		lsFileInfo{name: "a", size: 1, mode: 0644},
		lsFileInfo{name: "b", size: 2, mode: 0644},
	}
	l := &listerAt{entries: entries}

	// A buffer that is exactly filled reports no error yet.
	buf := make([]os.FileInfo, 1)
	n, err := l.ListAt(buf, 0)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "a", buf[0].Name())

	n, err = l.ListAt(buf, 1)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "b", buf[0].Name())

	// Reading past the end reports EOF.
	n, err = l.ListAt(buf, 2)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 0, n)

	// A buffer that is bigger than the entry list is filled partially and
	// then reports EOF.
	big := make([]os.FileInfo, 5)
	n, err = l.ListAt(big, 0)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 2, n)
	require.Equal(t, "a", big[0].Name())
	require.Equal(t, "b", big[1].Name())
}

func TestShellQuote(t *testing.T) {
	require.Equal(t, "'/tmp/plain'", shellQuote("/tmp/plain"))
	require.Equal(t, `'/tmp/with space'`, shellQuote("/tmp/with space"))
	require.Equal(t, `'/tmp/it'\''s'`, shellQuote("/tmp/it's"))
}
