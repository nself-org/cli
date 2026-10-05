package portable

import (
	"os"
	"time"
)

// fileID names a file on its volume: device and inode on Unix, volume serial
// number and file index on Windows. It is the zero value where the platform
// offers neither (then size and mtime are the only change signals).
type fileID struct{ dev, ino uint64 }

// snapshot is what a member looked like when it was verified. A later open
// must find the same file: the same fileID, size and modification time.
type snapshot struct {
	id   fileID
	size int64
	mod  time.Time
}

func (s snapshot) equal(o snapshot) bool {
	return s.id == o.id && s.size == o.size && s.mod.Equal(o.mod)
}

// snapshotOf reads the identity, size and mtime of an open file and its link
// count, all from the descriptor (never from the path).
func snapshotOf(fh *os.File) (snapshot, uint64, os.FileInfo, error) {
	fi, err := fh.Stat()
	if err != nil {
		return snapshot{}, 0, nil, err
	}
	id, nlink, err := handleInfo(fh, fi)
	if err != nil {
		return snapshot{}, 0, nil, err
	}
	return snapshot{id: id, size: fi.Size(), mod: fi.ModTime()}, nlink, fi, nil
}
