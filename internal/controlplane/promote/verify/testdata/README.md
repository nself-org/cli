# evidence verify goldens

Real stdout of `ci evidence verify --file <evidence.json> --at-revision <sha> --json`,
captured from the ci plugin (nself-org/plugins, free/ci, plugin version 1.2.13,
plugins commit 076f5063f245cbe0fcd2d05120eef2a477679e6c), built with
`go build -mod=vendor ./cmd` and run against a scratch git repository
(`NSELF_V15=1`, `NSELF_CI_REPO`, `NSELF_CI_HOME`, one `kind: test` job).

- `evidence-verify-pass.golden.json`: a passing default-class document (exit 0).
- `evidence-verify-release-class.golden.json`: the same document with its check
  kind edited to `release`, which the plugin classes as `release` (exit 11: the
  edit breaks the document's digests, so the plugin rejects it, but the report
  still carries the class). A passing release-class run needs a container
  executor, which the capture host did not have.
- `evidence-verify-rejected.golden.json`: a rejected document (exit 11), the
  error envelope.

`evidence_contract_test.go` feeds these through the same reader promote uses, so
a plugin rename of `data.class`, `data.result`, `data.quarantined` or
`error.message` fails here instead of turning every promote into E497.
To refresh after a plugin report change, rerun the command above and replace the files.
