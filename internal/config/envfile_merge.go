package config

// Purpose: read the same ordered env files used for Compose interpolation.
// Inputs: explicit file paths, in precedence order. Outputs: merged values.
// Constraints: no process exports or shell fallback; parsing errors redact values.

// ReadEnvFiles merges env files; later files win, including empty overrides.
func ReadEnvFiles(paths []string) (map[string]string, error) {
	merged := make(map[string]string)
	for _, path := range paths {
		values, err := ReadEnvFile(path)
		if err != nil {
			return nil, err
		}
		for key, value := range values {
			merged[key] = value
		}
	}
	return merged, nil
}
