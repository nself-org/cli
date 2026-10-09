package database

// Purpose: bounded Hasura metadata tar transport.
// Inputs: a metadata directory or a tar stream. Outputs: a tar stream or a
// replaced metadata directory. Constraints: validate before touching destination.
import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

const maxMetadataBytes int64 = 64 << 20
const maxMetadataEntries = 10000

var removeMetadataBackup = os.RemoveAll

// ExportMetadataArchive encodes live metadata as a self-contained config.yaml.
func ExportMetadataArchive(ctx context.Context, cfg *config.Config, w io.Writer) error {
	data, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	return packMetadataJSON(data, w)
}

func packMetadataJSON(data []byte, w io.Writer) error {
	var metadata interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("parse exported metadata: %w", err)
	}
	yml, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	if len(yml) > int(maxMetadataBytes) {
		return fmt.Errorf("metadata archive: exceeds 64 MiB")
	}
	tw := tar.NewWriter(w)
	if err := tw.WriteHeader(&tar.Header{Name: "metadata/config.yaml", Mode: 0644, Size: int64(len(yml)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := tw.Write(yml); err != nil {
		return err
	}
	return tw.Close()
}

// ApplyMetadataArchive applies an unpacked tree through the in-process
// include resolver, so a temporary archive needs no Hasura CLI project config.
func ApplyMetadataArchive(ctx context.Context, cfg *config.Config, metadataDir string) error {
	if err := validateArchiveIncludes(metadataDir); err != nil {
		return err
	}
	return applyViaIncludeResolver(ctx, cfg, metadataDir)
}

func validateArchiveIncludes(root string) error {
	graph := map[string][]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			idx := strings.Index(line, "!include ")
			if idx < 0 {
				continue
			}
			name := strings.TrimSpace(line[idx+len("!include "):])
			if name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
				return fmt.Errorf("metadata archive: invalid include %q", name)
			}
			target := filepath.Clean(filepath.Join(filepath.Dir(p), name))
			rel, err := filepath.Rel(root, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("metadata archive: include escapes metadata: %q", name)
			}
			graph[p] = append(graph[p], target)
		}
		return nil
	})
	if err != nil {
		return err
	}
	state := map[string]byte{}
	var visit func(string) error
	visit = func(p string) error {
		if state[p] == 1 {
			return fmt.Errorf("metadata archive: cyclic include %q", p)
		}
		if state[p] == 2 {
			return nil
		}
		state[p] = 1
		for _, child := range graph[p] {
			if err := visit(child); err != nil {
				return err
			}
		}
		state[p] = 2
		return nil
	}
	for p := range graph {
		if err := visit(p); err != nil {
			return err
		}
	}
	return nil
}

// Pack writes regular files beneath dir as metadata-relative tar entries.
func Pack(dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	count := 0
	var total int64
	err := filepath.WalkDir(dir, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == dir {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("metadata archive: non-regular file %q", p)
		}
		count++
		if count > maxMetadataEntries {
			return fmt.Errorf("metadata archive: too many entries")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > maxMetadataBytes {
			return fmt.Errorf("metadata archive: exceeds 64 MiB")
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		h := &tar.Header{Name: path.Join("metadata", filepath.ToSlash(rel)), Mode: 0644, Size: info.Size(), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = tw.Close()
		return err
	}
	return tw.Close()
}

// Unpack validates the entire stream in a private directory before replacing dir.
func Unpack(r io.Reader, dir string) error {
	parent := filepath.Dir(dir)
	stageParent := parent
	for {
		if _, statErr := os.Stat(stageParent); statErr == nil {
			break
		}
		if next := filepath.Dir(stageParent); next == stageParent {
			return fmt.Errorf("metadata archive: no existing parent for %s", dir)
		} else {
			stageParent = next
		}
	}
	stage, err := os.MkdirTemp(stageParent, ".hasura-metadata-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	tr := tar.NewReader(r)
	count := 0
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		if count > maxMetadataEntries {
			return fmt.Errorf("metadata archive: too many entries")
		}
		name := h.Name
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name || !strings.HasPrefix(name, "metadata/") || strings.Contains("/"+name+"/", "/../") || hasDotPathComponent(name) || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > maxMetadataBytes-total {
			return fmt.Errorf("metadata archive: invalid entry %q", name)
		}
		total += h.Size
		rel := strings.TrimPrefix(name, "metadata/")
		if rel == "" {
			return fmt.Errorf("metadata archive: empty path")
		}
		target := filepath.Join(stage, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("metadata archive: duplicate entry %q", name)
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(f, tr, h.Size)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if count == 0 {
		return fmt.Errorf("metadata archive: no regular files")
	}
	if err := os.Chmod(stage, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	backup := ""
	hadOld := false
	if _, err := os.Lstat(dir); err == nil {
		backup, err = os.MkdirTemp(parent, ".hasura-old-")
		if err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
		if err := os.Rename(dir, backup); err != nil {
			return err
		}
		hadOld = true
	}
	if err := os.Rename(stage, dir); err != nil {
		if hadOld {
			_ = os.Rename(backup, dir)
		}
		return err
	}
	if hadOld {
		if err := removeMetadataBackup(backup); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "metadata archive: remove old backup %s: %v\n", backup, err)
		}
	}
	return nil
}

func hasDotPathComponent(name string) bool {
	for _, component := range strings.Split(name, "/") {
		if strings.HasPrefix(component, ".") {
			return true
		}
	}
	return false
}
