//go:build windows

package maintenance

import (
	"fmt"
	"syscall"
	"unsafe"
)

// GetDiskUsage returns current disk utilisation for the C:\ drive on Windows.
func GetDiskUsage() (DiskUsage, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes, totalBytes, totalFreeBytes uint64
	root, _ := syscall.UTF16PtrFromString(`C:\`)
	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return DiskUsage{}, fmt.Errorf("GetDiskFreeSpaceExW: %w", err)
	}

	usedBytes := totalBytes - freeBytes
	const gb = 1024 * 1024 * 1024
	var usedPct int
	if totalBytes > 0 {
		usedPct = int((usedBytes * 100) / totalBytes)
	}
	return DiskUsage{
		UsedPercent: usedPct,
		TotalGB:     float64(totalBytes) / gb,
		UsedGB:      float64(usedBytes) / gb,
		FreeGB:      float64(freeBytes) / gb,
	}, nil
}

// DiskCleanupOptions configures a DiskCleanup run. RunnerRoots/Home/PressureThreshold
// are accepted for API parity with the POSIX build but are no-ops here — GitHub
// Actions runner-farm reclaim (job workspaces, go-build/grype/trivy exclusions) is
// POSIX-shaped and guarded out of the Windows build entirely rather than partially
// reimplemented against an untested Windows process/path model. Only the docker-prune
// safety fix applies on both platforms.
type DiskCleanupOptions struct {
	DryRun            bool
	PressureThreshold int
	Home              string
	RunnerRoots       []RunnerRoot
	SharedCacheRoots  []string
	UsageOverride     *DiskUsage
}

// RunnerRoot mirrors the POSIX type for API parity; unused on Windows.
type RunnerRoot struct {
	Name string
	Path string
}

// DiskCleanup runs the full cleanup with default options.
func DiskCleanup() CleanupResult {
	return DiskCleanupWithOptions(DiskCleanupOptions{})
}

// DiskCleanupDryRun runs the cleanup in dry-run mode.
func DiskCleanupDryRun() CleanupResult {
	return DiskCleanupWithOptions(DiskCleanupOptions{DryRun: true})
}

// DiskCleanupWithOptions runs the same docker-prune safety fix as the POSIX build
// (see dockerReclaim in disk_shared.go). Runner-farm and cache reclaim are not
// attempted on Windows and are reported as skipped for diagnosability.
func DiskCleanupWithOptions(opts DiskCleanupOptions) CleanupResult {
	result := CleanupResult{DryRun: opts.DryRun}
	var before DiskUsage
	if opts.UsageOverride != nil {
		before = *opts.UsageOverride
	} else {
		before, _ = GetDiskUsage()
	}
	result.Before = before

	dockerOut, dockerErrs := dockerReclaimFunc(opts.DryRun)
	result.DockerPruneOut = dockerOut
	result.Errors = append(result.Errors, dockerErrs...)

	result.Skipped = append(result.Skipped, SkipEntry{
		Path:   "runner workspaces / build caches",
		Reason: "runner-farm reclaim is not implemented on windows",
	})

	after, _ := GetDiskUsage()
	result.After = after
	return result
}
