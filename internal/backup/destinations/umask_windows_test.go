//go:build windows

package destinations

func syscallUmask(m int) int { return 0 }
