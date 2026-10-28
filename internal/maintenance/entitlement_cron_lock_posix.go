//go:build darwin || linux

package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func (h host) removeCron(hash string) (bool, error) {
	if h.goos != "linux" || !h.crontab {
		return false, nil
	}
	unlock, err := h.lockCron()
	if err != nil {
		return false, err
	}
	defer unlock()
	cur, err := h.cronList()
	if err != nil {
		return false, err
	}
	rest, tagged := taggedLines(cur, entCronTag+hash)
	if len(tagged) == 0 {
		return false, nil
	}
	out, err := h.run(strings.Join(rest, "\n")+"\n", "crontab", "-")
	if err != nil {
		return false, cmdErr(out, err, "crontab", "-")
	}
	return true, nil
}

// lockCron shares the lock with the cron self-removal shell. Never unlink it:
// waiters must continue to lock the same inode.
func (h host) lockCron() (func(), error) {
	dir := filepath.Join(h.home, ".nself")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "entitlement-cron.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
