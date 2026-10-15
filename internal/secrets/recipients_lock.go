package secrets

// Purpose: serialize recipient read-modify-audit-save across processes.
// Inputs: a sidecar path beside an existing encrypted store.
// Outputs: a release function, or an error before loading plaintext.
// Constraints: never unlink the sidecar; ciphertext is atomically replaced.
// SPORT: P7-TRUST-03, D-0579.

import "os"

func lockRecipientStore(path string) (func(), error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, decryptError()
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		_ = f.Close()
		return nil, decryptError()
	}
	if err := lockRecipientFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Closing releases the kernel lock even when the process exits early.
	return func() { _ = f.Close() }, nil
}
