package docker

import "strings"

// Purpose: retain project interpolation files across compose entry points.
// Inputs: the project directory and optional compose manifest paths.
// Outputs: a compose client with the generated interpolation environment.
// Constraints: use ProjectEnvFiles; never parse environment files here.
func NewProjectCompose(workdir string, files ...string) *Compose {
	c := NewCompose(files...)
	c.EnvFiles = ProjectEnvFiles(workdir)
	return c
}

// projectStreamArgs adds project interpolation flags to raw compose streams.
// Explicit --env-file flags retain the caller's chosen interpolation scope.
func projectStreamArgs(workdir string, args []string) []string {
	if len(args) == 0 || args[0] != "compose" {
		return args
	}
	if hasGlobalEnvFile(args[1:]) {
		return args
	}
	return append(NewProjectCompose(workdir).buildBaseArgs(), args[1:]...)
}

// hasGlobalEnvFile only considers Compose global options, before its command.
// An exec child's --env-file option must not change project interpolation.
func hasGlobalEnvFile(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--env-file" || strings.HasPrefix(arg, "--env-file=") {
			return true
		}
		if !strings.HasPrefix(arg, "-") || arg == "--" {
			break
		}
		switch arg {
		case "-f", "--file", "-p", "--project-name", "--project-directory", "--profile", "--parallel", "--progress", "--ansi":
			i++ // the next token is an option value, not the compose command
		}
	}
	return false
}
