package invoke

import (
	"strings"
	"testing"
)

func TestNullCheckLargeNumbers(t *testing.T) {
	for _, member := range []string{"args", "argv"} {
		for _, number := range []string{"1e9999", strings.Repeat("9", 400)} {
			_, err := DecodeRequest([]byte("{\"" + member + "\":[" + number + "]}"))
			if err == nil || strings.Contains(err.Error(), number) {
				t.Fatalf("%s: number leaked or accepted: %v", member, err)
			}
		}
	}
}
