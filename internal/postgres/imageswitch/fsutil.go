package imageswitch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// nonEmpty returns an error unless path is a file with content.
func nonEmpty(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s is missing", filepath.Base(path))
	}
	if fi.Size() == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(path))
	}
	return nil
}

// itoa renders an int.
func itoa(n int) string { return strconv.Itoa(n) }
