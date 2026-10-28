package backup

import (
	"context"
	"strings"
	"testing"
)

func TestStreamMissingDestinationNamesConfigDefault(t *testing.T) {
	cfg := minimalConfig()
	cfg.Backup.Remote = ""
	_, err := Stream(context.Background(), cfg, StreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "BACKUP_REMOTE") || strings.Contains(err.Error(), "NSELF_BACKUP_DESTINATION") {
		t.Fatalf("missing destination error = %v; want BACKUP_REMOTE", err)
	}
}
