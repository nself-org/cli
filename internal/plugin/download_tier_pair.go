package plugin

import "fmt"

// Tier-pair slugs share an API route. Name the resolved catalogue tier so a
// free request cannot accidentally select the licensed archive (or vice versa).
func tierPairDownloadURL(name string, paid bool) string {
	tier := "free"
	if paid {
		tier = "licensed"
	}
	return fmt.Sprintf("%s/plugins/%s/download?tier=%s", pingAPIURL(), name, tier)
}
