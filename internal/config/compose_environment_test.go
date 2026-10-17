package config

import (
	"errors"
	"os"
	"runtime"
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
					home := "HOME"
					if runtime.GOOS == "windows" {
						home = "USERPROFILE"
					}
					for _, key := range []string{"PATH", home, "DOCKER_HOST", "DOCKER_CONTEXT", "SSH_AUTH_SOCK"} {
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
				if key == "" { // Windows per-drive entries such as "=C:=C:\\" cannot be set by name
					continue
				}
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

func TestComposeRuntimeVariableNames(t *testing.T) {
	for _, key := range []string{"PATH", "DOCKER_HOST", "SSH_AUTH_SOCK"} {
		if !composeRuntimeVariable(key) {
			t.Errorf("%s must reach Compose", key)
		}
	}
	for _, key := range []string{"HASURA_GRAPHQL_JWT_SECRET", "COMPOSE_PROJECT_NAME", "POSTGRES_PASSWORD"} {
		if composeRuntimeVariable(key) {
			t.Errorf("%s must not reach Compose", key)
		}
	}
	if runtime.GOOS == "windows" {
		for _, key := range []string{"Path", "SystemRoot", "ComSpec"} {
			if !composeRuntimeVariable(key) {
				t.Errorf("%s must reach Compose on Windows", key)
			}
		}
	}
}
