# Symbol-level govulncheck findings only: the first trace frame names a
# function, i.e. the code actually calls the vulnerable symbol. Module-level
# findings (frames with a module but no function) are what text mode reports
# as "modules you require, but your code doesn't appear to call".
# Same semantics as .github/scripts/govuln-gate.sh (text mode).
select(.finding != null)
| .finding
| select(.trace != null and (.trace | length) > 0 and (.trace[0].function // "") != "")
| .osv
