# Maintenance safety

Deploy rollback treats an absent `.nself/state/deploy.json` as a fresh install.
An existing record must identify its schema version or legacy strategy. Empty
objects and JSON `null` are errors, so they cannot select legacy rollback.
Before remote image pulls, an inspect failure stops the apply unless Docker
explicitly reports that the image is absent. Invalid image IDs also stop the apply.

Operation locks invalidate their re-entry token before releasing the kernel
lock. A token from a different host cannot authorize re-entry. Unsupported
platform notices appear only when stderr is a terminal.

Plugin nginx validation ignores template examples in comments and permits
native uppercase nginx variables. Known unresolved template names still refuse
the snippet. Init container classification uses the effective Compose label
after applying overrides in file order.

Hasura metadata exports preserve strings with multiple trailing newlines through
the include layout. Hasura CLI projects may omit `metadata_directory` to use
`metadata`; directory aliases are compared by filesystem identity.

The affected-package helper scans owned data paths for readers outside their
owning package and includes importers when embedded data is deleted. The vendor
check preserves spaces in local replacement paths when scanning for symlinks.
