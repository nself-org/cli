package compose

// Purpose: resolve the group that owns the Docker socket, so a container which
// mounts the socket but runs as a non-root user can actually read it.
//
// Inputs:  the Docker socket path on the host.
// Outputs: dockerSocketGroup() -> (gid, ok).
// Constraints: must degrade silently. On Windows, on a host with no socket, or
// on any platform where the stat does not expose a gid, it returns ok=false and
// the caller emits no group_add at all — never a wrong or zero-value group.
// socketGidAt is implemented per-OS in docker_socket_group_unix.go and
// docker_socket_group_windows.go, mirroring compose_unix.go/compose_windows.go.
//
// WHY this exists: the admin service runs as user 1000:1000 and mounts
// /var/run/docker.sock. The socket is typically srw-rw---- root:root (or
// root:docker) mode 660, so uid 1000 is denied. `docker version` inside the
// container then fails with "permission denied while trying to connect to the
// docker API", the admin health endpoint reports dockerOk=false, and
// /api/health answers 503 — on every host, not just CI. The container's own
// healthcheck hits a different path and still reports "healthy", which is why
// this went unnoticed: `docker ps` shows the admin container as healthy while
// the admin UI cannot manage Docker at all.

// dockerSocketPath is the host path the admin service mounts.
const dockerSocketPath = "/var/run/docker.sock"

// dockerSocketGroup returns the numeric GID owning the Docker socket.
//
// ok is false when the socket does not exist or the platform does not report a
// gid. Callers must treat that as "add no group", not as gid 0: silently
// granting the root group on a host where the socket is absent would widen
// privileges for no reason.
func dockerSocketGroup() (string, bool) {
	return dockerSocketGroupAt(dockerSocketPath)
}

// dockerSocketGroupAt is dockerSocketGroup with an injectable path, so tests do
// not depend on the machine actually running Docker.
func dockerSocketGroupAt(path string) (string, bool) {
	return socketGidAt(path)
}
