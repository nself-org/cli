package output

// Exit classes of contract:cli.exit-codes v1, as written in the error
// object's class member.
const (
	ClassUser               = "user"
	ClassInfra              = "infra"
	ClassAuth               = "auth"
	ClassDestructiveBlocked = "destructive_blocked"
	ClassOther              = "other"
)

// Process exit statuses of contract:cli.exit-codes v1.
const (
	ExitUser               = 1
	ExitInfra              = 2
	ExitAuth               = 3
	ExitDestructiveBlocked = 4
)

// ExitCodeFor returns the process exit status of an exit class: user 1,
// infra 2, auth 3, destructive_blocked 4. "other" has no status of its own
// (it names any status above 4) and, like an unknown class, returns 1; a
// caller with a specific status sets ErrorDetail.ExitCode itself.
func ExitCodeFor(class string) int {
	switch class {
	case ClassInfra:
		return ExitInfra
	case ClassAuth:
		return ExitAuth
	case ClassDestructiveBlocked:
		return ExitDestructiveBlocked
	}
	return ExitUser
}

// ClassFor names the exit class of a process exit status; any status other
// than 1 to 4 is "other".
func ClassFor(exitCode int) string {
	switch exitCode {
	case ExitUser:
		return ClassUser
	case ExitInfra:
		return ClassInfra
	case ExitAuth:
		return ClassAuth
	case ExitDestructiveBlocked:
		return ClassDestructiveBlocked
	}
	return ClassOther
}
