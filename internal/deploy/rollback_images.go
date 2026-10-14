package deploy

// Purpose: preserve the running images inside each host's pre-deploy archive.
// Inputs: old compose manifests, container image IDs, and the recorded tar path.
// Outputs: per-service immutable IDs and a temporary rollback compose override.
// Constraints: validate all retained images before file/database restore; never
// pull or build on a rollback with recorded identities. The existing per-host
// tar reference in deploy and promote records owns this evidence too.

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const rollbackImagesFile = ".nself-rollback-images.json"

var rollbackServiceRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func captureRunningImages(ctx context.Context, r Remote, rp string) (map[string]string, error) {
	images := map[string]string{}
	files, err := remoteLines(ctx, r, path.Join(rp, ".nself/compose-files.txt"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		old, err := remoteLines(ctx, r, path.Join(rp, "docker-compose.yml"))
		if err != nil {
			return nil, err
		}
		if len(old) == 0 {
			return images, nil
		}
		files = []string{path.Join(rp, "docker-compose.yml")}
	}
	envs, err := remoteLines(ctx, r, path.Join(rp, ".nself/compose-env-files.txt"))
	if err != nil {
		return nil, err
	}
	if len(envs) == 0 {
		if old, err := remoteLines(ctx, r, path.Join(rp, ".env")); err != nil {
			return nil, err
		} else if len(old) > 0 {
			envs = []string{path.Join(rp, ".env")}
		}
	}
	base := composeBase(files, envs)
	out, err := r.Run(ctx, shScript(fmt.Sprintf("cd %s && %s config --services # rollback-image-capture", q(rp), base)))
	if err != nil {
		return nil, fmt.Errorf("reading previous compose services: %w", err)
	}
	for _, svc := range strings.Fields(string(out)) {
		if !rollbackServiceRE.MatchString(svc) {
			return nil, errs.New("E489", "unsafe service in previous compose; nothing was changed")
		}
		out, err = r.Run(ctx, shScript(fmt.Sprintf("cd %s && %s ps --status running --quiet -- %s", q(rp), base, q(svc))))
		if err != nil {
			return nil, fmt.Errorf("reading running containers for %s: %w", svc, err)
		}
		for _, ctr := range strings.Fields(string(out)) {
			id, err := r.Run(ctx, shScript(fmt.Sprintf("docker inspect --type container --format '{{.Image}}' %s", q(ctr))))
			if err != nil {
				return nil, fmt.Errorf("reading running image for %s: %w", svc, err)
			}
			value := strings.TrimSpace(string(id))
			if !imageIDRe.MatchString(value) || (images[svc] != "" && images[svc] != value) {
				return nil, errs.New("E489", "running service images are invalid or disagree; nothing was changed")
			}
			images[svc] = value
		}
	}
	return images, nil
}

func archiveImagesScript(tar string, images map[string]string) string {
	data, _ := json.Marshal(images)
	return fmt.Sprintf(`
	tmp=$(mktemp -d); new=; trap 'rm -f "$tmp/archive.tar" "$tmp/%s" "$new"; rmdir "$tmp"' EXIT
printf '%%s' %s > "$tmp/%s"
gzip -dc %s > "$tmp/archive.tar"
tar -rf "$tmp/archive.tar" -C "$tmp" %s
new=$(mktemp %s)
gzip -n -c "$tmp/archive.tar" > "$new"
tar -tzf "$new" >/dev/null
tar -xOzf "$new" %s | cmp - "$tmp/%s"
mv -f "$new" %s`, rollbackImagesFile, q(string(data)), rollbackImagesFile, q(tar), q(rollbackImagesFile), q(path.Join(path.Dir(tar), ".rollback-image-archive-XXXXXX")), q(rollbackImagesFile), rollbackImagesFile, q(tar))
}

// archiveRunningImages runs only after the database snapshot, before sync.
// The recorded archive path stays the same for promote's pre-write checkpoint.
func archiveRunningImages(ctx context.Context, r Remote, rp, tar string) error {
	images, err := captureRunningImages(ctx, r, rp)
	if err != nil {
		return err
	}
	_, err = r.Run(ctx, shScript("set -e; umask 077; "+archiveImagesScript(tar, images)))
	return err
}

func readRollbackImages(ctx context.Context, r Remote, rp, tar string) (map[string]string, error) {
	if !strings.HasPrefix(tar, path.Join(rp, BackupDir)+"/pre-deploy-") || strings.Contains(tar, "..") || !strings.HasSuffix(tar, ".tar.gz") {
		return nil, errs.New("E489", "unsafe image backup path; nothing was changed")
	}
	listing, err := r.Run(ctx, shScript(fmt.Sprintf("tar -tzf %s", q(tar))))
	if err != nil {
		return nil, err
	}
	found := false
	for _, file := range strings.Split(string(listing), "\n") {
		found = found || file == rollbackImagesFile
	}
	if !found {
		return nil, nil
	} // Legacy archives have no retained identities.
	out, err := r.Run(ctx, shScript(fmt.Sprintf("tar -xOzf %s %s", q(tar), q(rollbackImagesFile))))
	if err != nil {
		return nil, err
	}
	var images map[string]string
	if err := json.Unmarshal(out, &images); err != nil || images == nil {
		return nil, errs.New("E489", "invalid recorded image identities; nothing was changed")
	}
	for svc, id := range images {
		if !rollbackServiceRE.MatchString(svc) || !imageIDRe.MatchString(id) {
			return nil, errs.New("E489", "invalid recorded image identity; nothing was changed")
		}
		if _, err := r.Run(ctx, shScript("docker image inspect "+q(id)+" >/dev/null")); err != nil {
			return nil, errs.Wrap("E489", fmt.Sprintf("recorded image %s for %s is unavailable on %s; nothing was changed", id, svc, r.Label()), err)
		}
	}
	return images, nil
}

func prepareRollbackImages(ctx context.Context, o *ApplyOptions, model *ComposeModel) (func(), error) {
	noop := func() {}
	if len(o.rollbackImages) == 0 {
		return noop, nil
	}
	services := map[string]any{}
	for svc, id := range o.rollbackImages {
		services[svc] = map[string]string{"image": id, "pull_policy": "never"}
		if service, ok := model.Services[svc]; ok {
			service.Image = id
			model.Services[svc] = service
		}
	}
	data, _ := json.Marshal(map[string]any{"services": services})
	out, err := o.Remote.Run(ctx, shScript(fmt.Sprintf("set -e; umask 077; f=$(mktemp %s); printf '%%s' %s > \"$f\"; printf '%%s' \"$f\"", q(path.Join(o.RemotePath, BackupDir, ".rollback-images-XXXXXX")), q(string(data)))))
	if err != nil {
		return noop, err
	}
	file := string(out)
	if !strings.HasPrefix(file, path.Join(o.RemotePath, BackupDir)+"/.rollback-images-") || strings.ContainsAny(file, "\r\n") {
		return noop, errs.New("E489", "invalid rollback override path")
	}
	o.Set.Compose = append(append([]string{}, o.Set.Compose...), file)
	return func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		_, _ = o.Remote.Run(ctx, shScript("rm -f -- "+q(file)))
	}, nil
}
