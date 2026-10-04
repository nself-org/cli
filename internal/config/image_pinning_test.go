package config

import (
	"strings"
	"testing"
)

func TestValidateImagePinning(t *testing.T) {
	for v, ok := range map[string]bool{"": true, "lock": true, "legacy": true, " LOCK ": true, "Legacy": true, "lcok": false, "pinned": false, "1": false} {
		t.Setenv("IMAGE_PINNING", v)
		err := validateImagePinning()
		if (err == nil) != ok {
			t.Errorf("IMAGE_PINNING=%q: err = %v, want ok=%v", v, err, ok)
		}
		if err != nil && !strings.Contains(err.Error(), "IMAGE_PINNING") {
			t.Errorf("error %q must name IMAGE_PINNING", err)
		}
	}
}

// TestLoadRejectsInvalidImagePinning: an invalid value fails Load with a clear
// error instead of silently falling back to the default mode.
func TestLoadRejectsInvalidImagePinning(t *testing.T) {
	t.Setenv("IMAGE_PINNING", "lcok")
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "IMAGE_PINNING") {
		t.Errorf("Load err = %v, want an IMAGE_PINNING error", err)
	}
}
