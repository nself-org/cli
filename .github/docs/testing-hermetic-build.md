# Build and released-reader regression fixtures

The build sink regression runs under umask `077` and compares published files
against their planned modes. New artifacts keep their planned permissions;
rewrites preserve existing permissions.

The manifest migration reader scan uses complete reader-bearing source files
from release `v1.4.12`, commit
`1c1962daad278076f8125833f40b191e7eb237d5`. They live beneath
`tools/manifestv2migrate/testdata/v1412-source/` with `.txt` suffixes so the
historical packages do not compile into the current module. Eight files cover
every reader in `testdata/v1412-readers.txt`, including its shared type
definitions. Tests scan their
actual decoded fields and compare them with the current v2 manifest. They need
neither Git configuration nor tags nor network access. The mutation control
checks that adding a decoded field changes the scan.

Missing-entitlement tests isolate home directories, caches and all supported
license key variables, and remove external programs from PATH. Doctor timeout
tests allow two seconds for the child to start and retain a larger completion
bound below the deliberately slow child's lifetime.
