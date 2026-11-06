package pgstream

// Purpose: cover every stage of the pipe: native and rclone sources, age
// decryption, and the error each stage reports.
// Inputs: shell stubs for rclone and age on PATH; an in-memory load.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	errRemote  = errors.New("remote failed")
	errDecrypt = errors.New("decrypt failed")
)

// stubs writes shell scripts named by key onto a fresh PATH entry.
func stubs(t *testing.T, scripts map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stubs are shell scripts")
	}
	bin := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read broke") }
func (failingReader) Close() error             { return nil }

func run(t *testing.T, src Source) (string, error) {
	t.Helper()
	var got bytes.Buffer
	src.RemoteErr, src.DecryptErr = errRemote, errDecrypt
	if src.Load == nil {
		src.Load = func(_ context.Context, r io.Reader) error {
			_, err := io.Copy(&got, r)
			return err
		}
	}
	err := Stream(context.Background(), src)
	return got.String(), err
}

func TestStreamNative(t *testing.T) {
	got, err := run(t, Source{Native: io.NopCloser(strings.NewReader("dump"))})
	if err != nil || got != "dump" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := run(t, Source{Native: failingReader{}}); !errors.Is(err, errRemote) {
		t.Fatalf("native read failure: %v", err)
	}
	loadErr := errors.New("load failed")
	_, err = run(t, Source{Native: io.NopCloser(strings.NewReader("dump")), Load: func(context.Context, io.Reader) error { return loadErr }})
	if !errors.Is(err, loadErr) {
		t.Fatalf("load failure: %v", err)
	}
}

func TestStreamRclone(t *testing.T) {
	stubs(t, map[string]string{"rclone": "[ \"$1\" = cat ] && [ \"$2\" = r2:b/ok ] && printf dump && exit 0\necho 'object not found' >&2\nexit 1\n"})
	got, err := run(t, Source{From: "r2:b/ok"})
	if err != nil || got != "dump" {
		t.Fatalf("got %q err %v", got, err)
	}
	_, err = run(t, Source{From: "r2:b/missing"})
	if !errors.Is(err, errRemote) || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("rclone failure: %v", err)
	}
}

func TestStreamRcloneSilentFailure(t *testing.T) {
	stubs(t, map[string]string{"rclone": "exit 3\n"})
	if _, err := run(t, Source{From: "r2:b/x"}); !errors.Is(err, errRemote) {
		t.Fatalf("silent rclone failure: %v", err)
	}
}

func TestStreamDecrypt(t *testing.T) {
	stubs(t, map[string]string{"age": "[ \"$3\" = good.key ] && exec cat\ncat >/dev/null\necho 'no identity matched' >&2\nexit 1\n"})
	got, err := run(t, Source{Native: io.NopCloser(strings.NewReader("plain")), Encrypted: true, AgeKey: "good.key"})
	if err != nil || got != "plain" {
		t.Fatalf("got %q err %v", got, err)
	}
	_, err = run(t, Source{Native: io.NopCloser(strings.NewReader("x")), Encrypted: true, AgeKey: "bad.key"})
	if !errors.Is(err, errDecrypt) || !strings.Contains(err.Error(), "no identity matched") {
		t.Fatalf("age failure: %v", err)
	}
}

func TestStreamDecryptSilentFailure(t *testing.T) {
	stubs(t, map[string]string{"age": "cat >/dev/null\nexit 2\n"})
	if _, err := run(t, Source{Native: io.NopCloser(strings.NewReader("x")), Encrypted: true, AgeKey: "k"}); !errors.Is(err, errDecrypt) {
		t.Fatalf("silent age failure: %v", err)
	}
}

func TestStreamMissingBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := run(t, Source{From: "r2:b/x"}); err == nil || !strings.Contains(err.Error(), "rclone") {
		t.Fatalf("missing rclone: %v", err)
	}
	// Which stage reports first is not fixed here (the download also sees the
	// closed pipe), only that the stream fails.
	if _, err := run(t, Source{Native: io.NopCloser(strings.NewReader("x")), Encrypted: true, AgeKey: "k"}); err == nil {
		t.Fatal("missing age: stream succeeded")
	}
}
