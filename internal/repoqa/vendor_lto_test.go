package repoqa

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
)

const ltoChunkSize = 32 * 1024

// scanLTO keeps enough overlap to find either section name across reads.
func scanLTO(r io.Reader) (bool, error) {
	buf := make([]byte, ltoChunkSize+8)
	keep := 0
	for {
		n, err := r.Read(buf[keep:])
		data := buf[:keep+n]
		if bytes.Contains(data, []byte(".llvmbc")) || bytes.Contains(data, []byte(".llvmcmd")) {
			return true, nil
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		keep = len(data)
		if keep > 8 {
			keep = 8
		}
		copy(buf, data[len(data)-keep:])
	}
}

// checkLTOArchives compares both directions, so upstream fixes retire the list.
func checkLTOArchives(root string, listed []string) error {
	want := make(map[string]bool)
	for _, path := range listed {
		want[path] = true
	}
	var problems []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".a") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		has, scanErr := scanLTO(f)
		closeErr := f.Close()
		if scanErr != nil {
			return fmt.Errorf("%s: %w", rel, scanErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if has && !want[rel] {
			problems = append(problems, rel+": unlisted archive has LTO bitcode")
		}
		if !has && want[rel] {
			problems = append(problems, rel+": listed archive is clean; remove entry and strip step")
		}
		delete(want, rel)
		return nil
	})
	if err != nil {
		return err
	}
	for path := range want {
		problems = append(problems, path+": listed archive is missing")
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}

func TestVendoredArchivesWithLTOBitcodeAreListed(t *testing.T) {
	data, err := os.ReadFile("testdata/lto_archives.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkLTOArchives("../../vendor", strings.Fields(string(data))); err != nil {
		t.Fatal(err)
	}
}

func TestLTOArchiveCheckFlagsUnlistedArchive(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"new.a": ".llvmbc", "clean.a": "machine code"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := checkLTOArchives(root, []string{"clean.a"})
	if err == nil || !strings.Contains(err.Error(), "new.a") || !strings.Contains(err.Error(), "clean.a") {
		t.Fatalf("want both mismatched paths, got %v", err)
	}
	t.Run("unreadable symlink", func(t *testing.T) {
		// A dangling archive symlink is unreadable even when tests run as root.
		if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "unreadable.a")); err != nil {
			// Windows ERROR_PRIVILEGE_NOT_HELD is not os.ErrPermission.
			if os.IsPermission(err) || errors.Is(err, errors.ErrUnsupported) ||
				(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314))) {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
			t.Fatal(err)
		}
		err := checkLTOArchives(root, nil)
		if err == nil || !strings.Contains(err.Error(), "unreadable.a") {
			t.Fatalf("unreadable archive must fail, got %v", err)
		}
	})
}

func TestLTOScannerFindsNameAcrossChunkBorder(t *testing.T) {
	for _, name := range []string{".llvmbc", ".llvmcmd"} {
		data := append(bytes.Repeat([]byte{'x'}, ltoChunkSize+5), []byte(name)...)
		for _, tc := range []struct {
			data []byte
			want bool
		}{{data, true}, {bytes.Repeat([]byte{'x'}, 2*ltoChunkSize), false}, {[]byte(name), true}, {[]byte("short"), false}} {
			got, err := scanLTO(bytes.NewReader(tc.data))
			if err != nil || got != tc.want {
				t.Fatalf("scan %s: got %v, %v; want %v", name, got, err, tc.want)
			}
		}
	}
}
