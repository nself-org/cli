package docker

// compose_hooks.go — hooks that run before a compose down.
//
// Purpose: a fronted project's network cannot be removed while the shared host
// proxy's nginx is attached to it (P7-ADOPT-21), so every compose-down path of
// the project must detach first. internal/docker cannot import the code that
// knows the proxy layout (internal/sharedproxy imports this package), so that
// code registers a hook from cmd at init and ComposeDown runs it.
// Inputs: a hook function; the compose workdir ComposeDown acts on.
// Outputs: RegisterPreDownHook returns the function that unregisters the hook.
// Constraints: a hook cannot fail the down (it reports through its own
// channel); hooks run in registration order, once per ComposeDown call, before
// docker is invoked; safe for concurrent use.

import (
	"context"
	"sync"
)

// PreDownHook runs before `docker compose down` in workdir.
type PreDownHook func(ctx context.Context, workdir string)

type preDownEntry struct {
	id   int
	hook PreDownHook
}

var (
	preDownMu    sync.Mutex
	preDownSeq   int
	preDownHooks []preDownEntry
)

// RegisterPreDownHook adds h to the hooks ComposeDown runs and returns the
// function that removes it again.
func RegisterPreDownHook(h PreDownHook) (unregister func()) {
	preDownMu.Lock()
	defer preDownMu.Unlock()
	preDownSeq++
	id := preDownSeq
	preDownHooks = append(preDownHooks, preDownEntry{id: id, hook: h})
	return func() {
		preDownMu.Lock()
		defer preDownMu.Unlock()
		for i, e := range preDownHooks {
			if e.id == id {
				preDownHooks = append(preDownHooks[:i:i], preDownHooks[i+1:]...)
				return
			}
		}
	}
}

// RunPreDownHooks runs every registered hook for workdir.
func RunPreDownHooks(ctx context.Context, workdir string) {
	preDownMu.Lock()
	hooks := make([]PreDownHook, 0, len(preDownHooks))
	for _, e := range preDownHooks {
		hooks = append(hooks, e.hook)
	}
	preDownMu.Unlock()
	for _, h := range hooks {
		h(ctx, workdir)
	}
}
