package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// apiDocsSitePath names the served docs file. Fronted builds use the same
// project prefix as sharedSink; unfronted builds retain their existing name.
func apiDocsSitePath(sitesDir, frontedBy, project string) string {
	name := "api-docs.conf"
	if frontedBy != "" {
		name = project + "." + name
	}
	return filepath.Join(sitesDir, name)
}

// removeAPIDocsSiteConf removes disabled fronted docs through the build sink,
// so plans and confirmed builds include the removal. Only our exact ownership
// marker authorizes removal; legacy unowned and operator-managed files survive.
func (st *buildState) removeAPIDocsSiteConf() error {
	if st.cfg.Nginx.FrontedBy == "" {
		return nil // the own-directory generation sweep already removed it
	}
	dir, err := resolveNginxSitesDir(st.workdir, st.cfg.Nginx.FrontedBy)
	if err != nil {
		return err
	}
	path := apiDocsSitePath(dir, st.cfg.Nginx.FrontedBy, st.cfg.ProjectName)
	data, err := st.sink.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading disabled api-docs nginx conf: %w", err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if first != sharedMarker(st.cfg.ProjectName) {
		return nil
	}
	if err := st.siteSink().Remove(path); err != nil {
		return fmt.Errorf("removing disabled api-docs nginx conf: %w", err)
	}
	return nil
}
