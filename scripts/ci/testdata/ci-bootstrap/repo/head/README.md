# fixture repo

The Go manifest selects the Go stack. The pinned synthetic shell gate at
`scripts/ci/vendor-sync-check.sh` checks the source and deliberately exits 1
with `probe fixture: expected gate failure`. It needs no Go toolchain.
The probe requires that gate's FAIL row, marker, and overall failed verdict;
matching startup errors or skipped tools alone cannot pass parity.
