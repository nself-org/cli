package nself

// Purpose: stage a verified bundle without rewriting any byte.
// Inputs: an opened reader and an empty stage path. Outputs: manifest.json and every member.
// Constraints: private modes (dirs 0700, files 0600); members stream through the checking reader.
import (
	"context"
	"errors"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
	"io"
	"os"
	"path/filepath"
)

func stageError() error { return errs.New("E520", "bundle staging failed") }

func copyBundle(ctx context.Context, r *portable.Reader, stage string) error {
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return stageError()
	}
	// The manifest is copied verbatim, never re-marshalled, so the hash of what was verified is what loads.
	if err := copyManifest(ctx, filepath.Join(r.Dir(), portable.ManifestName), filepath.Join(stage, portable.ManifestName)); err != nil {
		return stageError()
	}
	for _, f := range r.Manifest().Files {
		if err := ctx.Err(); err != nil {
			return stageError()
		}
		src, err := r.Open(f.Path)
		if err != nil {
			return err
		}
		err = writeMember(ctx, src, filepath.Join(stage, filepath.FromSlash(f.Path)))
		if err != nil {
			return err
		}
	}
	return nil
}

// writeMember copies src to dst and closes src. A read that ends in a checksum failure keeps its
// E516 error; every other failure is a staging failure.
func writeMember(ctx context.Context, src io.ReadCloser, dst string) error {
	defer func() { _ = src.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return stageError()
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return stageError()
	}
	_, err = io.Copy(out, ctxReader{ctx, src})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		var cli *errs.CLIError
		if errors.As(err, &cli) && cli.Code == "E516" {
			return err
		}
		return stageError()
	}
	return nil
}

// ctxReader stops a copy between chunks once the context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// copyManifest copies manifest.json when it is still the regular file the reader verified: the
// opened handle must be the same file as the lstat entry (a swap for a symlink or another file
// is refused) and at most MaxManifestBytes are read.
func copyManifest(ctx context.Context, src, dst string) error {
	want, err := os.Lstat(src)
	if err != nil || !want.Mode().IsRegular() || want.Size() > portable.DefaultLimits.MaxManifestBytes {
		return errors.New("manifest is not a regular file within limits")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if got, e := in.Stat(); e != nil || !os.SameFile(want, got) {
		return errors.New("manifest changed while it was staged")
	}
	return writeMember(ctx, io.NopCloser(io.LimitReader(in, portable.DefaultLimits.MaxManifestBytes)), dst)
}
