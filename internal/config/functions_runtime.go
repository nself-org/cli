package config

import "fmt"

// Purpose: reject runtime typos before the functions image is selected.
// Inputs: FUNCTIONS_RUNTIME as read by the loader.
// Outputs: an error for unsupported runtimes.
// Constraints: empty selects the node default; names are case sensitive.
func validateFunctionsRuntime(runtime string) error {
	switch runtime {
	case "", "node", "deno", "python":
		return nil
	default:
		return fmt.Errorf("FUNCTIONS_RUNTIME=%q is not valid: set it to node, deno, or python, or unset it for the node default", runtime)
	}
}
