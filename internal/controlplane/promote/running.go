package promote

import "fmt"

// MatchRunning requires one running identity for each compared service.
func MatchRunning(rows []DigestRow, images []SourceImage) error {
	if len(rows) == 0 {
		return fmt.Errorf("no compared image identities")
	}
	running := make(map[string]string, len(images))
	for _, image := range images {
		running[image.Service] = image.Digest
	}
	for _, row := range rows {
		if row.Status == "unverifiable" {
			delete(running, row.Service)
			continue
		}
		digest, found := running[row.Service]
		if !found || row.Target == "" || digest != row.Target || row.Source != row.Target {
			return fmt.Errorf("service %s running digest %s differs from compared digest %s", row.Service, digest, row.Target)
		}
		delete(running, row.Service)
	}
	if len(running) > 0 {
		return fmt.Errorf("running release has uncompared services")
	}
	return nil
}
