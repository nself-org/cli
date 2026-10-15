package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// pluginTestFallbackImage is used when the plugin's go.mod names no usable Go
// version. Free plugins declare go 1.25.0 to 1.26.4.
const pluginTestFallbackImage = "golang:1.26-alpine"

var (
	goModGoLine        = regexp.MustCompile(`^go\s+(\d+)\.(\d+)`)
	goModToolchainLine = regexp.MustCompile(`^toolchain\s+go(\d+)\.(\d+)`)
)

// pluginUnitTestImage returns the golang image that can build the plugin at
// pluginPath. Official golang images set GOTOOLCHAIN=local, so the image must be
// at least as new as the go directive (and toolchain line, when present) in
// go.mod. A missing or unreadable go.mod yields pluginTestFallbackImage.
func pluginUnitTestImage(pluginPath string) string {
	data, err := os.ReadFile(filepath.Join(pluginPath, "go.mod"))
	if err != nil {
		return pluginTestFallbackImage
	}
	maj, min := -1, -1
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		for _, re := range []*regexp.Regexp{goModGoLine, goModToolchainLine} {
			m := re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			if a > maj || (a == maj && b > min) {
				maj, min = a, b
			}
		}
	}
	if maj < 0 {
		return pluginTestFallbackImage
	}
	return fmt.Sprintf("golang:%d.%d-alpine", maj, min)
}

// pluginUnitTestDockerCmd builds the docker run command for the plugin's unit
// tests, using the image chosen by pluginUnitTestImage.
func pluginUnitTestDockerCmd(pluginPath string) *exec.Cmd {
	return exec.Command("docker", "run", "--rm",
		"-v", pluginPath+":/workspace",
		"-w", "/workspace",
		pluginUnitTestImage(pluginPath),
		"sh", "-c", "go test ./... -v -count=1",
	)
}
