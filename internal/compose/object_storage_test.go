package compose

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

func storageFixture(t *testing.T, minio, object, custom bool) *config.Config {
	t.Helper()
	t.Setenv("OBJECT_STORAGE_ENABLED", "false")
	t.Setenv("OBJECT_STORAGE_BUCKETS", "uploads,assets")
	t.Setenv("SEAWEEDFS_PORT", "")
	t.Setenv("SEAWEEDFS_VERSION", "")
	if object {
		t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	}
	cfg := minimalConfig()
	cfg.ProjectName = "storage-fixture"
	cfg.Minio.Enabled = minio
	cfg.Minio.RootUser = "test-access-key"
	cfg.Minio.RootPassword = "fake-secret-not-real-0000"
	cfg.Minio.S3AccessKey = "test-access-key"
	cfg.Minio.S3SecretKey = "fake-secret-not-real-0000"
	if custom {
		cfg.CustomServices = []config.CustomService{{Name: "worker", Port: 3000}}
	}
	return cfg
}

func storageGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "object_storage", name+".yaml")
	if os.Getenv("UPDATE_OBJECT_STORAGE_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("compose differs from base golden %s", path)
	}
}

// TestSeaweedV14Unchanged records the base generator before the v1.5 implementation.
func TestSeaweedV14Unchanged(t *testing.T) {
	storagePluginSeam(t, false)
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range []struct {
			name                  string
			minio, object, custom bool
		}{
			{"minio", true, false, false}, {"object", false, true, false},
			{"none", false, false, false}, {"custom", true, false, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := storageFixture(t, tc.minio, tc.object, tc.custom)
				got, err := NewGenerator(cfg).Generate()
				if compat.V15() {
					if tc.minio {
						if err == nil {
							t.Fatal("v1.5 MinIO without plugin must fail")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if bytes.Contains(got, []byte("    seaweedfs:")) != tc.object {
						t.Fatal("v1.5 store selection mismatch")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				storageGolden(t, "v14-"+tc.name, got)
			})
		}
	})
}

func storagePluginSeam(t *testing.T, installed bool) {
	t.Helper()
	old := minioPluginInstalled
	minioPluginInstalled = func() bool { return installed }
	t.Cleanup(func() { minioPluginInstalled = old })
}

func TestObjectStorageSeaweedGenerated(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	cfg := storageFixture(t, false, true, true)
	dir := t.TempDir()
	g := NewGenerator(cfg).WithWorkDir(dir)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatal(err)
	}
	svc, ok := dc.Services["seaweedfs"]
	if !ok {
		t.Fatal("SeaweedFS missing")
	}
	if _, ok := dc.Services["seaweedfs-init"]; !ok {
		t.Fatal("bucket init missing")
	}
	if _, ok := dc.Services["minio"]; ok {
		t.Fatal("core MinIO generated")
	}
	if _, ok := LockedRef("seaweedfs"); !ok {
		t.Fatal("seaweedfs has no lock entry")
	}
	if want := DefaultImage("seaweedfs"); svc.Image != want || dc.Services["seaweedfs-init"].Image != want {
		t.Fatalf("image=%q init=%q lock=%q", svc.Image, dc.Services["seaweedfs-init"].Image, want)
	}
	wantHC := []string{"CMD", "wget", "-q", "--spider", "http://127.0.0.1:8333/healthz"}
	if !reflect.DeepEqual(svc.Healthcheck.Test, wantHC) {
		t.Fatalf("healthcheck=%v", svc.Healthcheck)
	}
	for _, key := range []string{"S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		if svc.Environment[key] != "${"+key+"}" {
			t.Fatalf("credential %s is not a reference: %q", key, svc.Environment[key])
		}
	}
	if !reflect.DeepEqual(svc.Volumes, []string{"seaweedfs_data:/data"}) {
		t.Fatalf("volumes=%v", svc.Volumes)
	}
	if _, ok := dc.Volumes["seaweedfs_data"]; !ok {
		t.Fatal("named volume missing")
	}
	cmd := strings.Join(svc.Command.([]string), "\n")
	for _, flag := range []string{"-ip=127.0.0.1", "-ip.bind=127.0.0.1", "-s3.ip.bind=0.0.0.0", "-s3.port=8333"} {
		if !strings.Contains(cmd, flag) {
			t.Fatalf("server command lacks %s", flag)
		}
	}
	init := dc.Services["seaweedfs-init"]
	if init.NetworkMode != "service:seaweedfs" || len(init.Networks) != 0 || len(init.Ports) != 0 {
		t.Fatalf("init network=%q networks=%v ports=%v, want service:seaweedfs with none", init.NetworkMode, init.Networks, init.Ports)
	}
	if !strings.Contains(strings.Join(init.Command.([]string), "\n"), "-master=127.0.0.1:9333") || strings.Contains(strings.Join(init.Command.([]string), "\n"), "seaweedfs:9333") {
		t.Fatal("init must reach the master on loopback")
	}
	if svc.User != "1000:1000" {
		t.Fatalf("user=%q, want the image seaweed account", svc.User)
	}
	if !containsString(svc.Tmpfs, "/tmp") {
		t.Fatal("identity needs tmpfs")
	}
	got, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(got, &doc); err != nil {
		t.Fatal(err)
	}
	initSvc, ok := doc.Services["seaweedfs-init"]
	if !ok {
		t.Fatal("rendered compose has no seaweedfs-init")
	}
	if initSvc["network_mode"] != "service:seaweedfs" {
		t.Fatalf("rendered network_mode=%v, want service:seaweedfs", initSvc["network_mode"])
	}
	for _, key := range []string{"networks", "ports"} {
		if _, has := initSvc[key]; has {
			t.Fatalf("rendered seaweedfs-init has a %s key", key)
		}
	}
	for _, value := range []string{cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey} {
		if bytes.Contains(got, []byte(value)) {
			t.Fatalf("compose contains fake credential")
		}
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == "s3.json" {
			t.Errorf("identity written in project: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestMinioWithoutPluginE138(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	for _, core := range []bool{false, true} {
		t.Run(map[bool]string{false: "minio-only", true: "minio-and-core"}[core], func(t *testing.T) {
			cfg := storageFixture(t, true, core, false)
			t.Setenv("MINIO_ENABLED", "true")
			_, err := NewGenerator(cfg).Generate()
			var cliErr *errs.CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != "E138" || errs.ExitCodeFor(err) != 1 ||
				!strings.Contains(err.Error(), "nself add minio") || !strings.Contains(err.Error(), "OBJECT_STORAGE_ENABLED=true") {
				t.Fatalf("want E138 with both remedies and exit 1, got %v", err)
			}
			if _, err := NewGeneratorWithProfile(cfg, ProfileOps).Generate(); err != nil {
				t.Fatalf("ops profile must gate MinIO intent before E138: %v", err)
			}
			storagePluginSeam(t, true)
			dc, err := NewGenerator(cfg).buildDockerCompose()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := dc.Services["minio"]; ok {
				t.Fatal("plugin must supply MinIO")
			}
		})
	}
	t.Run("loaded-s3-only", func(t *testing.T) {
		storageFixture(t, false, false, false)
		for _, key := range []string{"MINIO_ENABLED", "STORAGE_ENABLED", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"} {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
		for _, key := range []string{"ENV", "PROJECT_NAME", "BASE_DOMAIN", "OBJECT_STORAGE_ENABLED", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
			t.Setenv(key, "")
		}
		dir := t.TempDir()
		env := "ENV=dev\nPROJECT_NAME=storage-fixture\nBASE_DOMAIN=localhost\nOBJECT_STORAGE_ENABLED=true\nS3_ACCESS_KEY=test-access-key\nS3_SECRET_KEY=fake-secret-not-real-0000\n"
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Minio.Enabled {
			t.Fatal("core S3 identity must not infer MinIO intent")
		}
		dc, err := NewGenerator(cfg).WithWorkDir(dir).buildDockerCompose()
		if err != nil {
			t.Fatalf("core S3 identity must not trigger E138: %v", err)
		}
		if _, ok := dc.Services["seaweedfs"]; !ok {
			t.Fatal("loaded core S3 identity must generate SeaweedFS")
		}
	})
}

func TestSeaweedPortLoopback(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	for _, tc := range []struct{ name, port, want string }{
		{"port-default", "", "127.0.0.1:8333:8333"}, {"port-override", "18333", "127.0.0.1:18333:8333"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := storageFixture(t, false, true, false)
			t.Setenv("SEAWEEDFS_PORT", tc.port)
			dc, err := NewGenerator(cfg).buildDockerCompose()
			if err != nil {
				t.Fatal(err)
			}
			ports := dc.Services["seaweedfs"].Ports
			if !reflect.DeepEqual(ports, []string{tc.want}) {
				t.Fatalf("ports=%v", ports)
			}
			got, err := yaml.Marshal(ports)
			if err != nil {
				t.Fatal(err)
			}
			storageGolden(t, tc.name, got)
			if bytes.Contains(got, []byte("0.0.0.0")) {
				t.Fatal("wildcard publish")
			}
		})
	}
	for _, port := range []string{"abc", "0", "-1", "65536"} {
		cfg := storageFixture(t, false, true, false)
		t.Setenv("SEAWEEDFS_PORT", port)
		if _, err := NewGenerator(cfg).Generate(); err == nil || !strings.Contains(err.Error(), "SEAWEEDFS_PORT") {
			t.Fatalf("port %q error=%v", port, err)
		}
	}
}

func TestS3EndpointCustomServices(t *testing.T) {
	compattest.Set(t, true)
	for _, tc := range []struct {
		name                  string
		object, plugin, minio bool
		endpoint              string
	}{
		{"core-wins", true, true, true, "http://seaweedfs:8333"},
		{"core-only", true, false, false, "http://seaweedfs:8333"},
		{"plugin-with-minio-intent", false, true, true, "http://minio:9000"},
		{"plugin-without-minio-intent", false, true, false, ""},
		{"none", false, false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storagePluginSeam(t, tc.plugin)
			cfg := storageFixture(t, tc.minio, tc.object, true)
			dc, err := NewGenerator(cfg).buildDockerCompose()
			if err != nil {
				t.Fatal(err)
			}
			env := dc.Services["worker"].Environment
			if env["S3_ENDPOINT"] != tc.endpoint {
				t.Fatalf("S3_ENDPOINT=%q want %q", env["S3_ENDPOINT"], tc.endpoint)
			}
			if tc.endpoint == "" {
				for _, key := range []string{"S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_BUCKET"} {
					if _, ok := env[key]; ok {
						t.Fatalf("%s injected without a store", key)
					}
				}
			}
		})
	}
	t.Run("no-object-storage-profile", func(t *testing.T) {
		storagePluginSeam(t, true)
		cfg := storageFixture(t, true, true, true)
		cs := config.CustomService{Name: "worker", Port: 3000}
		if env := coreEnvVarsFor(cfg, cs, nil, true); env["S3_ENDPOINT"] == "" {
			t.Fatal("storage profile must inject the endpoint")
		}
		for key := range coreEnvVarsFor(cfg, cs, nil, false) {
			if strings.HasPrefix(key, "S3_") {
				t.Fatalf("profile without object storage injected %s", key)
			}
		}
	})
}

func TestSeaweedVersionOverride(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	cfg := storageFixture(t, false, true, false)
	t.Setenv("SEAWEEDFS_VERSION", "4.49")
	dc, err := NewGenerator(cfg).buildDockerCompose()
	if err != nil {
		t.Fatal(err)
	}
	want := ResolveImage("seaweedfs", ImageRef("seaweedfs", "4.49"))
	for _, name := range []string{"seaweedfs", "seaweedfs-init"} {
		if got := dc.Services[name].Image; got != want {
			t.Errorf("%s image=%q, want version override %q", name, got, want)
		}
	}
}

func TestSeaweedBucketScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the container script with a POSIX /bin/sh")
	}
	for _, tc := range []struct {
		name, buckets, existing, fail, drop, wantCreates string
		wantFailure                                      bool
	}{
		{name: "empty"},
		{name: "missing", buckets: "uploads,assets", wantCreates: "uploads\nassets\n"},
		{name: "existing-formatted-list", buckets: "uploads,assets", existing: "uploads\nassets\n"},
		{name: "create-failure", buckets: "uploads", fail: "uploads", wantFailure: true, wantCreates: "uploads\n"},
		{name: "create-lost", buckets: "uploads", drop: "uploads", wantFailure: true, wantCreates: "uploads\n"},
		{name: "invalid-name", buckets: "uploads,Bad_Name", wantFailure: true},
		{name: "invalid-short", buckets: "ab", wantFailure: true},
		{name: "invalid-long", buckets: strings.Repeat("a", 64), wantFailure: true},
		{name: "invalid-dots", buckets: "bad..name", wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			state, creates := filepath.Join(dir, "state"), filepath.Join(dir, "creates")
			for path, content := range map[string]string{state: tc.existing, creates: ""} {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			mock := `#!/bin/sh
set -eu
read -r command
case "$command" in
  s3.bucket.list)
    while IFS= read -r bucket; do
      printf '  %s\tsize:0\tlogical:0\tchunk:0\n' "$bucket"
    done < "$MOCK_STATE" ;;
  's3.bucket.create -name '*)
    bucket=${command#s3.bucket.create -name }
    printf '%s\n' "$bucket" >> "$MOCK_CREATES"
    if [ "$bucket" = "$MOCK_FAIL" ]; then
      printf '%s\n' 'error: bucket creation failed' >&2
      exit 1
    fi
    if [ "$bucket" = "$MOCK_DROP" ]; then
      exit 0
    fi
    if grep -F -x "$bucket" "$MOCK_STATE" >/dev/null; then
      printf '%s\n' 'error: bucket already exists' >&2
      exit 1
    fi
    printf '%s\n' "$bucket" >> "$MOCK_STATE" ;;
  *) exit 2 ;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "weed"), []byte(mock), 0700); err != nil {
				t.Fatal(err)
			}
			script := strings.ReplaceAll(seaweedBucketScript, "$$", "$")
			script = strings.ReplaceAll(script, "/tmp/buckets", filepath.Join(dir, "bucket-list"))
			run := func() {
				t.Helper()
				cmd := exec.Command("/bin/sh", "-c", script)
				cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "OBJECT_STORAGE_BUCKETS="+tc.buckets,
					"MOCK_STATE="+state, "MOCK_CREATES="+creates, "MOCK_FAIL="+tc.fail, "MOCK_DROP="+tc.drop)
				output, err := cmd.CombinedOutput()
				if (err != nil) != tc.wantFailure {
					t.Fatalf("script failure=%v, want %v: %s", err, tc.wantFailure, output)
				}
			}
			run()
			if !tc.wantFailure {
				run() // Existing buckets must also be harmless on a second startup.
			}
			got, err := os.ReadFile(creates)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantCreates {
				t.Fatalf("create attempts=%q, want %q", got, tc.wantCreates)
			}
		})
	}
}

// TestSeaweedServerScript runs the container entry script with a stub weed:
// bad credentials stop it before any identity file exists; good ones render
// the identity into the tmpfs path and never put a key on weed's argv.
func TestSeaweedServerScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the container script with a POSIX /bin/sh")
	}
	for _, tc := range []struct {
		name, access, secret string
		wantFailure          bool
	}{
		{"valid", "test-access-key", "fake-secret-not-real-0000", false},
		{"empty-access", "", "fake-secret-not-real-0000", true},
		{"empty-secret", "test-access-key", "", true},
		{"quote", "test-access-key", `fake"secret`, true},
		{"backslash", `test\key`, "fake-secret-not-real-0000", true},
		{"newline", "test-access-key", "fake\nsecret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			argv := filepath.Join(dir, "argv")
			stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$STUB_ARGV\"\n"
			if err := os.WriteFile(filepath.Join(dir, "weed"), []byte(stub), 0700); err != nil {
				t.Fatal(err)
			}
			identity := filepath.Join(dir, "s3.json")
			script := strings.ReplaceAll(seaweedServerScript, "$$", "$")
			script = strings.ReplaceAll(script, "/tmp/s3.json", identity)
			cmd := exec.Command("/bin/sh", "-c", script)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "STUB_ARGV="+argv,
				"S3_ACCESS_KEY="+tc.access, "S3_SECRET_KEY="+tc.secret)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantFailure {
				t.Fatalf("failure=%v, want %v: %s", err, tc.wantFailure, out)
			}
			if tc.wantFailure {
				if _, statErr := os.Stat(identity); statErr == nil {
					t.Fatal("identity written despite bad credentials")
				}
				return
			}
			args, err := os.ReadFile(argv)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(args), tc.access) || strings.Contains(string(args), tc.secret) {
				t.Fatal("credential on weed argv")
			}
			ident, err := os.ReadFile(identity)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(ident), `"accessKey":"`+tc.access+`"`) || !strings.Contains(string(ident), `"secretKey":"`+tc.secret+`"`) {
				t.Fatalf("identity missing credentials: %s", ident)
			}
		})
	}
}

// TestSeaweedCredentialsE142 checks the build-time identity check: a missing or unusable
// key stops the build with E142 naming the variable, never its value.
func TestSeaweedCredentialsE142(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	for _, tc := range []struct{ name, access, secret, variable string }{
		{"missing-access-key", "", "fake-secret-not-real-0000", "S3_ACCESS_KEY"},
		{"missing-secret-key", "test-access-key", "", "S3_SECRET_KEY"},
		{"quote", "test-access-key", `fake"secret-not-real`, "S3_SECRET_KEY"},
		{"backslash", `test\access-key`, "fake-secret-not-real-0000", "S3_ACCESS_KEY"},
		{"control", "test-access-key", "fake\tsecret-not-real", "S3_SECRET_KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := storageFixture(t, false, true, false)
			cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey = tc.access, tc.secret
			_, err := NewGenerator(cfg).Generate()
			var cliErr *errs.CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != "E142" || errs.ExitCodeFor(err) != 1 {
				t.Fatalf("want E142 with exit 1, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.variable) {
				t.Fatalf("error does not name %s: %v", tc.variable, err)
			}
			for _, value := range []string{tc.access, tc.secret} {
				if value != "" && strings.Contains(err.Error(), value) {
					t.Fatal("error contains a credential value")
				}
			}
		})
	}
	t.Run("profile-without-object-storage", func(t *testing.T) {
		cfg := storageFixture(t, false, true, false)
		cfg.Minio.S3AccessKey = ""
		if _, err := NewGeneratorWithProfile(cfg, ProfileOps).Generate(); err != nil {
			t.Fatalf("ops profile must not check object storage keys: %v", err)
		}
	})
}

// TestMinioE138RemedyTwo applies the second E138 remedy to a v1.4 demo-shaped env (MinIO
// credentials still present): core object storage on, MinIO off, and the build passes.
func TestMinioE138RemedyTwo(t *testing.T) {
	compattest.Set(t, true)
	storagePluginSeam(t, false)
	for _, key := range []string{"MINIO_ENABLED", "STORAGE_ENABLED", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "S3_ACCESS_KEY", "S3_SECRET_KEY", "OBJECT_STORAGE_ENABLED",
		"ENV", "PROJECT_NAME", "BASE_DOMAIN"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	base := "ENV=dev\nPROJECT_NAME=storage-fixture\nBASE_DOMAIN=localhost\nMINIO_ROOT_USER=nself\nMINIO_ROOT_PASSWORD=fake-secret-not-real-0000\n"
	load := func(extra string) (*config.Config, error) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(base+extra), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewGenerator(cfg).WithWorkDir(dir).Generate()
		for _, key := range []string{"MINIO_ENABLED", "OBJECT_STORAGE_ENABLED", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
			_ = os.Unsetenv(key)
		}
		return cfg, err
	}
	if _, err := load("MINIO_ENABLED=true\n"); err == nil || !strings.Contains(err.Error(), "E138") {
		t.Fatalf("demo-shaped v1.4 env must raise E138 in v1.5, got %v", err)
	}
	if _, err := load("MINIO_ENABLED=false\nOBJECT_STORAGE_ENABLED=true\nS3_ACCESS_KEY=test-access-key\nS3_SECRET_KEY=fake-secret-not-real-0000\n"); err != nil {
		t.Fatalf("remedy 2 must clear E138: %v", err)
	}
	if _, err := load("MINIO_ENABLED=false\n"); err != nil {
		t.Fatalf("remedy 3 must clear E138: %v", err)
	}
}
