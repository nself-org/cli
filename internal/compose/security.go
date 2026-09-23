package compose

// ServiceSecurity holds Docker security configuration for a service.
type ServiceSecurity struct {
	CapDrop     []string
	CapAdd      []string
	SecurityOpt []string
	ReadOnly    bool
	Tmpfs       []string
	User        string
}

// DefaultSecurity returns the security config applied to most services.
// read_only root FS with /tmp and /run as tmpfs.
func DefaultSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		Tmpfs:       []string{"/tmp", "/run"},
	}
}

// InitContainerSecurity returns the security config for ownership-fixing
// init containers (meilisearch-init). They run as root with CapDrop ALL, and
// "chown -R 1000:1000 /data; chmod -R 755 /data" then fails with "Operation
// not permitted" on any data dir it did not create, which aborted nself start
// and left nginx created but never started (nself-web prod, 2026-09-23).
// CHOWN changes owners, FOWNER lets chmod act on files owned by another uid,
// DAC_READ_SEARCH lets root descend into a 0700 directory once it no longer
// owns it. Verified minimal on a Linux volume: dropping any one fails.
func InitContainerSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "FOWNER", "DAC_READ_SEARCH"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		Tmpfs:       []string{"/tmp", "/run"},
	}
}

// PostgresSecurity returns the security config for the PostgreSQL service.
// PostgreSQL needs IPC_LOCK for shared memory and CHOWN/SETUID/SETGID for
// initdb. Root FS is read-only; data dir is a writable volume.
func PostgresSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "SETUID", "SETGID", "IPC_LOCK"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		Tmpfs:       []string{"/tmp", "/run/postgresql"},
	}
}

// MinioSecurity returns the security config for the MinIO service.
// MinIO needs CHOWN/SETUID/SETGID for data directory ownership.
func MinioSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "SETUID", "SETGID"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		Tmpfs:       []string{"/tmp", "/run"},
	}
}

// NginxSecurity returns the security config for the Nginx service.
// Nginx needs NET_BIND_SERVICE for ports 80/443 when running non-root.
//
// DAC_READ_SEARCH is required because CapDrop: ALL takes away root's usual
// ability to ignore file permissions, and the TLS material is bind-mounted
// from the host with host ownership. privkey.pem is written 0600 by
// mkcert/openssl and owned by whoever ran `nself build`, so without this the
// master cannot open it and nginx exits with
//
//	[emerg] cannot load certificate ".../fullchain.pem": Permission denied
//
// then crash-loops. The alternative was relaxing the private key to be
// world-readable on the host, which is a worse trade: this grants read and
// traverse bypass to one container that must read TLS material at startup,
// whereas DAC_OVERRIDE would also grant write bypass, and a 0644 key would
// expose it to every user on the machine.
func NginxSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop: []string{"ALL"},
		// The exact set a root-master nginx needs at startup, and nothing more.
		// CapDrop: ALL means root here has no privileges except these.
		//
		//   NET_BIND_SERVICE  bind 80 and 443
		//   DAC_READ_SEARCH   read the bind-mounted TLS material, which carries
		//                     host ownership and a 0600 private key
		//   CHOWN             hand the worker scratch dirs to uid 101 after
		//                     creating them (chown("/var/cache/nginx/client_temp", 101))
		//   SETUID / SETGID   fork the workers as the nginx user, which is what
		//                     `user nginx;` in the generated config asks for
		//
		// Each was found by nginx refusing to start without it, one per run.
		// DAC_OVERRIDE is deliberately NOT here: DAC_READ_SEARCH covers reading
		// and traversing, which is all the TLS material needs, while
		// DAC_OVERRIDE would also grant write bypass across the filesystem.
		CapAdd:      []string{"NET_BIND_SERVICE", "DAC_READ_SEARCH", "CHOWN", "SETUID", "SETGID"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		// nginx tmpfs handled separately in service builder (uid/gid specific)
	}
}

// RedisSecurity returns the security config for the Redis service.
func RedisSecurity() ServiceSecurity {
	return ServiceSecurity{
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges:true"},
		ReadOnly:    true,
		Tmpfs:       []string{"/tmp"},
	}
}

// applySecurityToService merges a ServiceSecurity into a ServiceConfig.
func applySecurityToService(svc *ServiceConfig, sec ServiceSecurity) {
	svc.CapDrop = sec.CapDrop
	if len(sec.CapAdd) > 0 {
		svc.CapAdd = sec.CapAdd
	}
	svc.SecurityOpt = sec.SecurityOpt
	if sec.ReadOnly {
		svc.ReadOnly = true
	}
	// Merge tmpfs: keep any existing tmpfs entries and add security ones.
	if len(sec.Tmpfs) > 0 {
		existing := make(map[string]bool)
		for _, t := range svc.Tmpfs {
			existing[t] = true
		}
		for _, t := range sec.Tmpfs {
			if !existing[t] {
				svc.Tmpfs = append(svc.Tmpfs, t)
			}
		}
	}
}
