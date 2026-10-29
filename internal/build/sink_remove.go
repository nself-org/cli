package build

// Cleanup removals use an open root so swapped paths cannot escape it.
import "os"

func (d *diskSink) RemoveWithin(root *os.Root, relative, _ string) error {
	return root.Remove(relative)
}

func (m *memSink) RemoveWithin(_ *os.Root, _ string, p string) error {
	return m.Remove(p)
}
