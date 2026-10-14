package entitlement

import (
	"errors"
	"reflect"
	"testing"
)

type recordingWriter struct {
	calls                          []string
	data                           []byte
	length                         int64
	writeErr, truncateErr, syncErr error
}

func (w *recordingWriter) Write(data []byte) (int, error) {
	w.calls = append(w.calls, "write")
	w.data = append([]byte(nil), data...)
	return len(data), w.writeErr
}

func (w *recordingWriter) Truncate(length int64) error {
	w.calls = append(w.calls, "truncate")
	w.length = length
	return w.truncateErr
}

func (w *recordingWriter) Sync() error {
	w.calls = append(w.calls, "sync")
	return w.syncErr
}

func TestWriteContentsErrorOrdering(t *testing.T) {
	failure := errors.New("storage failure")
	for _, tc := range []struct {
		name   string
		writer recordingWriter
		calls  []string
		want   error
	}{
		{"success", recordingWriter{}, []string{"write", "truncate", "sync"}, nil},
		{"write", recordingWriter{writeErr: failure}, []string{"write"}, failure},
		{"truncate", recordingWriter{truncateErr: failure}, []string{"write", "truncate"}, failure},
		{"sync", recordingWriter{syncErr: failure}, []string{"write", "truncate", "sync"}, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("replacement")
			err := writeContents(&tc.writer, data)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(tc.writer.calls, tc.calls) {
				t.Fatalf("got error=%v calls=%v; want error=%v calls=%v", err, tc.writer.calls, tc.want, tc.calls)
			}
			if string(tc.writer.data) != string(data) || (tc.name != "write" && tc.writer.length != int64(len(data))) {
				t.Fatalf("incorrect bytes or truncation length: %q %d", tc.writer.data, tc.writer.length)
			}
		})
	}
}
