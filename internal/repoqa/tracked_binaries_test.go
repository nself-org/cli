package repoqa

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPrefixLen is how much of each tracked file is inspected; it matches the
// 8000 bytes git itself scans for its binary heuristic.
const binaryPrefixLen = 8000

// classifyBinary reports what kind of compiled binary a tracked file is, or ""
// when it is not one.
//
// Purpose:     the EPIC G6 tracked-binary rule: a file fails when its first
//
//	bytes are an ELF, Mach-O (32/64-bit, either byte order, fat) or
//	PE header, or when its git mode is 100755 and its prefix holds a
//	NUL byte (catches executables whose magic is missing).
//
// Inputs:      mode, the git index mode ("100644", "100755", ...); prefix, up
//
//	to the first 8000 bytes of the file.
//
// Outputs:     "ELF", "Mach-O", "PE", "executable with NUL bytes", or "".
//
// Note: the fat magic ca fe ba be is also the header of a Java class file. No
// class file is tracked today; the rule stays exact and a legitimate class
// fixture must be generated at test time like any other binary fixture.
func classifyBinary(mode string, prefix []byte) string {
	if len(prefix) >= 4 {
		magic := prefix[:4]
		switch {
		case bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'}):
			return "ELF"
		case bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}),
			bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}),
			bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}),
			bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}),
			bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}),
			bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}):
			return "Mach-O"
		}
	}
	if len(prefix) >= 0x40 && prefix[0] == 'M' && prefix[1] == 'Z' {
		off := int(binary.LittleEndian.Uint32(prefix[0x3c:0x40]))
		if off >= 0 && off+4 <= len(prefix) && bytes.Equal(prefix[off:off+4], []byte{'P', 'E', 0, 0}) {
			return "PE"
		}
	}
	if mode == "100755" && bytes.IndexByte(prefix, 0) >= 0 {
		return "executable with NUL bytes"
	}
	return ""
}

// trackedFile is one entry of `git ls-files -z -s`.
type trackedFile struct{ mode, path string }

// parseLsFiles splits `git ls-files -z -s` output ("<mode> <sha> <stage>\t<path>\0").
func parseLsFiles(out []byte) []trackedFile {
	var files []trackedFile
	for _, rec := range bytes.Split(out, []byte{0}) {
		meta, path, ok := strings.Cut(string(rec), "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 3 {
			continue
		}
		files = append(files, trackedFile{mode: fields[0], path: path})
	}
	return files
}

// TestTrackedBinaries fails for every tracked file (vendor included) that is a
// compiled binary per classifyBinary. There is no allowlist: a binary fixture
// is generated at test time (Constitution section 2.6).
func TestTrackedBinaries(t *testing.T) {
	root := repoRoot(t)
	requireTool(t, "git")
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is not a git checkout (required in CI)", root)
		}
		t.Skip("not a git checkout")
	}

	cmd := exec.Command("git", "ls-files", "-z", "-s")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}

	for _, f := range parseLsFiles(out) {
		if f.mode != "100644" && f.mode != "100755" {
			continue // symlinks and submodule links carry no file content
		}
		fh, err := os.Open(filepath.Join(root, filepath.FromSlash(f.path)))
		if err != nil {
			continue // removed in the working tree: nothing to inspect
		}
		buf := make([]byte, binaryPrefixLen)
		n, _ := io.ReadFull(fh, buf)
		fh.Close()
		if kind := classifyBinary(f.mode, buf[:n]); kind != "" {
			t.Errorf("%s: tracked compiled binary (%s); remove it with git rm --cached and ignore it", f.path, kind)
		}
	}
}

// TestTrackedBinariesClassify pins the classifier on synthetic headers so the
// rule is proven without tracking a binary.
func TestTrackedBinariesClassify(t *testing.T) {
	pe := make([]byte, 0x80)
	copy(pe, "MZ")
	binary.LittleEndian.PutUint32(pe[0x3c:], 0x40)
	copy(pe[0x40:], []byte{'P', 'E', 0, 0})
	mzOnly := make([]byte, 0x80)
	copy(mzOnly, "MZ")

	cases := []struct {
		name   string
		mode   string
		prefix []byte
		want   string
	}{
		{"ELF", "100644", []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, "ELF"},
		{"Mach-O 64 little endian", "100644", []byte{0xcf, 0xfa, 0xed, 0xfe, 7, 0}, "Mach-O"},
		{"Mach-O 32 big endian", "100644", []byte{0xfe, 0xed, 0xfa, 0xce, 0, 0}, "Mach-O"},
		{"fat Mach-O", "100644", []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0}, "Mach-O"},
		{"PE", "100644", pe, "PE"},
		{"MZ without PE signature", "100644", mzOnly, ""},
		{"executable with NUL", "100755", []byte("ab\x00cd"), "executable with NUL bytes"},
		{"non-executable with NUL", "100644", []byte("ab\x00cd"), ""},
		{"executable script", "100755", []byte("#!/bin/sh\necho hi\n"), ""},
		{"empty", "100755", nil, ""},
	}
	for _, tc := range cases {
		if got := classifyBinary(tc.mode, tc.prefix); got != tc.want {
			t.Errorf("%s: classifyBinary = %q, want %q", tc.name, got, tc.want)
		}
	}
}
