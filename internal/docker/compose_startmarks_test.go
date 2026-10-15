package docker

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestStartMarksVanishedContainer checks that a removed container is omitted
// while surviving marks remain intact and other inspect failures still fail.
func TestStartMarksVanishedContainer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stderr  string
		wantErr bool
	}{
		{"no such object", "Error: No such object: gone", false},
		{"no such container", "Error: No such container: gone", false},
		{"daemon failure", "Cannot connect to the Docker daemon", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, dir := scriptDocker(t, `if [ "$1" = inspect ]; then
  if [ "$5" = gone ]; then
    echo '`+tc.stderr+`' >&2
    exit 1
  fi
  echo "start-$5"
  exit 0
fi
echo '[{"ID":"before","State":"exited"},{"ID":"gone","State":"exited"},{"ID":"after","State":"exited"}]'
`)
			marks, err := c.StartMarks(context.Background(), dir, "init")
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "docker inspect gone") || marks != nil {
					t.Fatalf("marks = %v, err = %v; want nil marks and inspect error", marks, err)
				}
				return
			}
			want := map[string]string{"before": "start-before", "after": "start-after"}
			if err != nil || !reflect.DeepEqual(marks, want) {
				t.Fatalf("marks = %v, err = %v; want %v", marks, err, want)
			}
		})
	}
}
