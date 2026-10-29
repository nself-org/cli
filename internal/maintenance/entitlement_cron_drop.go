package maintenance

import "fmt"

// cronDropCmd returns the shell that removes the tagged crontab line. The lock (fd 9) spans the
// whole read-modify-write, so a concurrent install is never overwritten with a stale read. Without
// flock the removal runs unlocked so the job does not run forever.
func cronDropCmd(tag string) string {
	return fmt.Sprintf(`{ mkdir -p "$HOME/.nself" && { if command -v flock >/dev/null 2>&1; then flock -x 9; fi; t=$(crontab -l 2>/dev/null) && printf '%%s\n' "$t" | grep -vF %s | crontab -; } 9>"$HOME/.nself/entitlement-cron.lock"; }`, shQuote(tag))
}
