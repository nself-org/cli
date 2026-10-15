//go:build !unix && !windows

package portable

import "os"

// Platforms without directory-handle-relative, no-follow opens fail closed.
func openMemberDirectory(string) (*os.File, error) { return nil, os.ErrPermission }

func openMemberAt(*os.File, string, bool) (*os.File, error) { return nil, os.ErrPermission }
