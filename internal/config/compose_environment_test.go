package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestComposeEnvironmentRestoresOnFailureAndPanic(t *testing.T) {
	t.Setenv("HASURA_GRAPHQL_JWT_SECRET", "caller-secret")
	t.Setenv("COMPOSE_PROJECT_NAME", "caller-project")
	t.Setenv("DOCKER_HOST", "unix:///test/docker.sock")
	t.Setenv("DOCKER_CONTEXT", "test-context")
	t.Setenv("SSH_AUTH_SOCK", "/test/agent.sock")
	for _, mode := range []string{"error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			before := os.Environ()
			errSentinel := errors.New("compose failure")
			func() {
				defer func() {
					if mode == "panic" && recover() != errSentinel {
						t.Error("callback panic was not propagated")
					}
				}()
				err := WithComposeEnvironment(func() error {
					for _, key := range []string{"HASURA_GRAPHQL_JWT_SECRET", "COMPOSE_PROJECT_NAME"} {
						if _, exists := os.LookupEnv(key); exists {
							t.Errorf("project variable %s leaked into Compose", key)
						}
					}
					for _, key := range []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "SSH_AUTH_SOCK"} {
						if os.Getenv(key) == "" {
							t.Errorf("runtime variable %s was removed", key)
						}
					}
					_ = os.Setenv("CALLBACK_TEMP_VALUE", "temporary")
					if mode == "panic" {
						panic(errSentinel)
					}
					return errSentinel
				})
				if err != errSentinel {
					t.Error("callback error was not propagated")
				}
			}()
			for _, entry := range before {
				key, value, _ := strings.Cut(entry, "=")
				if got, exists := os.LookupEnv(key); !exists || got != value {
					t.Errorf("caller environment was not restored for %s", key)
				}
			}
			if _, exists := os.LookupEnv("CALLBACK_TEMP_VALUE"); exists {
				t.Error("callback environment change escaped")
			}
		})
	}
}
