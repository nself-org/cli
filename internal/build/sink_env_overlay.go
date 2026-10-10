package build

// sink_env_overlay.go — the plan-mode sink with a pending env change
// (P7-LIVE-05).
//
// Purpose: `nself config set` plans the regeneration a new value causes before
// it writes the env line. BuildOptions.EnvOverlay carries the would-be bytes of
// the changed env file; config.Load reads them (LoadOptions.EnvOverlay) and so
// does every step that reads an env file through the sink (the .env.secrets
// append, the compose env-file manifest, the 0600 permission fix). The render
// is therefore the one the build will produce once the line is written.
// Inputs: the memSink of a plan-mode build and the overlay (project-relative
// env file name -> bytes). Outputs: reads and stats that see the overlay.
// Constraints: plan mode only (Build refuses an overlay in write mode). A file
// the build itself plans or removes is answered by memSink as before. An
// overlaid file reports mode 0600: `config set` enforces 0600 on every write.
// Directory listings are not overlaid: the build lists only nginx/sites
// through the sink, and env files live at the project root.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// envOverlayPerm is the mode of an env file after `config set` wrote it.
const envOverlayPerm fs.FileMode = 0o600

// envOverlaySink is memSink with pending env files in front of the disk.
type envOverlaySink struct {
	*memSink
	env map[string][]byte // canonical key -> pending bytes
}

// newEnvOverlaySink wraps m; overlay keys are project-relative file names.
func newEnvOverlaySink(m *memSink, overlay map[string][]byte) *envOverlaySink {
	s := &envOverlaySink{memSink: m, env: map[string][]byte{}}
	for name, data := range overlay {
		s.env[m.key(name)] = data
	}
	return s
}

// pending returns the overlay bytes of p unless the build planned or removed p.
func (s *envOverlaySink) pending(p string) ([]byte, bool) {
	k := s.key(p)
	if _, planned := s.files[k]; planned || s.removed[k] {
		return nil, false
	}
	data, ok := s.env[k]
	return data, ok
}

func (s *envOverlaySink) ReadFile(p string) ([]byte, error) {
	if data, ok := s.pending(p); ok {
		return append([]byte(nil), data...), nil
	}
	return s.memSink.ReadFile(p)
}

func (s *envOverlaySink) Stat(p string) (fs.FileInfo, error) {
	if data, ok := s.pending(p); ok {
		return memInfo{name: filepath.Base(p), size: int64(len(data)), mode: envOverlayPerm}, nil
	}
	return s.memSink.Stat(p)
}

// WriteFile keeps the mode of an overlaid file as os.WriteFile keeps the mode
// of a file that exists (memSink.WriteFile would read it from the disk copy).
func (s *envOverlaySink) WriteFile(p string, data []byte, perm fs.FileMode) error {
	if _, ok := s.pending(p); ok {
		return s.memSink.WriteAtomic(p, data, envOverlayPerm)
	}
	return s.memSink.WriteFile(p, data, perm)
}

// envLayerBytes reads one env file for a scan of its keys: the pending bytes
// in a plan with an overlay, else the file on disk.
func envLayerBytes(sink Sink, p string) ([]byte, error) {
	if s, ok := sink.(*envOverlaySink); ok {
		if data, ok := s.pending(p); ok {
			return data, nil
		}
	}
	return os.ReadFile(p)
}

// planSink returns the memSink behind a plan-mode sink, or nil in write mode.
func planSink(s Sink) *memSink {
	switch v := s.(type) {
	case *memSink:
		return v
	case *envOverlaySink:
		return v.memSink
	}
	return nil
}

// errEnvOverlayWrite refuses an overlay outside plan mode: a write build must
// read the env files it writes against, so the change is written first.
var errEnvOverlayWrite = fmt.Errorf("an env overlay is a plan-mode input: write the env file, then build")
