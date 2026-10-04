package main

// resolve.go — turn Sources into a LockFile through an Indexer.
//
// Purpose: the network half of `imagelock -resolve`: read each reference's
// index, record its digest and per-platform digests, and prove the digest
// addresses the index by reading `repository@digest` back.
// Inputs: []Source and an Indexer (the docker funnel in production, a fake in
// tests).
// Outputs: a *compose.LockFile sorted by name.
// Constraints: an entry that publishes no index, misses a required platform,
// or whose pinned digest differs from the registry's is an error naming it.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
)

// Indexer returns the raw index JSON of an image reference.
type Indexer interface {
	Index(ctx context.Context, ref string) ([]byte, error)
}

type dockerIndexer struct{}

func (dockerIndexer) Index(ctx context.Context, ref string) ([]byte, error) {
	return docker.ImageIndexRaw(ctx, ref)
}

// Resolve builds the lock for sources.
func Resolve(ctx context.Context, sources []Source, ix Indexer) (*compose.LockFile, error) {
	lf := &compose.LockFile{Generated: compose.LockHeader, SchemaVersion: compose.LockSchemaVersion}
	for _, s := range sources {
		ref := s.Repository + ":" + s.Version
		if s.Digest != "" {
			ref += "@" + s.Digest
		}
		info, err := indexOf(ctx, ix, ref)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", s.Name, ref, err)
		}
		if s.Digest != "" && s.Digest != info.Digest {
			return nil, fmt.Errorf("%s: images.json pins %s but the registry serves %s", s.Name, s.Digest, info.Digest)
		}
		back, err := indexOf(ctx, ix, s.Repository+"@"+info.Digest)
		if err != nil || back.Digest != info.Digest {
			return nil, fmt.Errorf("%s: digest %s does not address the index it was read from (%v)", s.Name, info.Digest, err)
		}
		want := s.Platforms
		if len(want) == 0 {
			want = []string{"linux/amd64"}
		}
		for _, p := range want {
			if info.Platforms[p] == "" {
				return nil, fmt.Errorf("%s (%s): publishes no %s manifest (has %v); confirm with the lead before locking", s.Name, ref, p, platformNames(info.Platforms))
			}
		}
		var mirror *string
		if s.Role != compose.RolePlugin {
			m := "docker.io/nself/" + s.Name
			mirror = &m
		}
		lf.Images = append(lf.Images, compose.Ref{Name: s.Name, Role: s.Role, Repository: s.Repository, Version: s.Version,
			IndexDigest: info.Digest, Platforms: info.Platforms, LegacyRef: s.LegacyRef, License: s.License, Source: s.Source, Mirror: mirror})
	}
	sort.Slice(lf.Images, func(i, j int) bool { return lf.Images[i].Name < lf.Images[j].Name })
	return lf, nil
}

func indexOf(ctx context.Context, ix Indexer, ref string) (docker.IndexInfo, error) {
	raw, err := ix.Index(ctx, ref)
	if err != nil {
		return docker.IndexInfo{}, err
	}
	info, err := docker.ParseIndex(raw)
	if errors.Is(err, docker.ErrNotIndex) {
		return info, fmt.Errorf("publishes no digest-addressable index: %w", err)
	}
	return info, err
}

func platformNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
