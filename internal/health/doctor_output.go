package health

import "strings"

// DoctorFailedService names the first service `nself doctor` output reports
// as unhealthy or failed, or "unknown".
func DoctorFailedService(out string) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "unhealthy") || strings.Contains(line, "failed") {
			if parts := strings.Fields(line); len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return "unknown"
}
