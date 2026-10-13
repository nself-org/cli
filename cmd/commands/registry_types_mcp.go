package commands

// Purpose: register `mcp setup`'s envelope schema. Inputs: none.
// Outputs: mcp setup data schema {client, path, content, written}.
// Constraints: available in both mode trees.
func init() { registerJSONType("mcp setup", mcpSetupResult{}) }
