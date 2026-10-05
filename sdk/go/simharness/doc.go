// Package simharness starts small fleets of Docker sshd containers for
// integration tests, and injects the faults the node and deploy scenarios
// need.
//
// Purpose:
//
//	One shared harness for every test that needs "a few remote hosts": the CLI
//	control-plane simulation (internal/controlplane/sim) and the ci plugin's
//	node scenarios. It generalises the older cli harness with digest-pinned
//	images, unique container and network names per Start call, and Exec,
//	CopyTo, Restart, Pause, Netem and Partition helpers.
//
// Rules:
//
//   - Opt-in. Start skips the test unless INTEGRATION=1. With INTEGRATION=1 and
//     no reachable Docker daemon it fails the test; it never skips.
//   - Parallel safe. Every Start picks a random fleet id; the network is
//     nself-sim-<id>, each container nself-sim-<id>-<name>. Every object
//     carries the labels org.nself.simharness=1 and
//     org.nself.simharness.fleet=<id>.
//   - Always cleaned up. Close runs from t.Cleanup (so also after t.Fatal) and
//     from a SIGINT/SIGTERM handler while a fleet is live. Close removes only
//     the containers and the network of its own fleet and never anything else.
//   - Pinned images. Every image reference is digest-pinned or built from a
//     digest-pinned base (testdata/Dockerfile.*). Start refuses a reference
//     without a sha256 digest.
//   - Least privilege. Nothing runs --privileged. Netem needs the node to
//     declare NET_ADMIN in NodeSpec.CapAdd. Ports are published on 127.0.0.1.
//   - Every docker process is created in docker.go from an argv slice; no
//     shell runs on the host.
//
// Requirements: a Linux Docker Engine for evidence (Docker Desktop and Colima
// run the same containers but are not release evidence), the docker CLI, and
// for Netem an image with tc (the debian, fedora and alpine images have it).
//
// Constraints: standard library only; no cgo.
package simharness
