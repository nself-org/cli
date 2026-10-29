package acme

// GenerationOwner hands root-issued Traefik certificates to the serving uid.
// Lookup stays lazy so other providers never read the host-owner metadata.
func GenerationOwner(provider string, euid int, workdir string, lookup func(string) (int, int)) *FileOwner {
	if provider != "traefik" || euid != 0 {
		return nil
	}
	uid, gid := lookup(workdir)
	if uid < 0 {
		return nil
	}
	return &FileOwner{UID: uid, GID: gid}
}
