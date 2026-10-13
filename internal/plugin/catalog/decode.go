package catalog

// decode.go — strict decoding of the catalog document.
//
// Purpose: turn a response body into a *Catalog, or fail with E116 when the
// document cannot be trusted as a catalog (not JSON, wrong schema_version, no
// plugins, a row without slug or with an unknown tier, a duplicate row).
// Inputs: the raw body. Outputs: *Catalog or a coded error. It never panics on
// any input (FuzzDecodeCatalog).

import (
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/errs"
)

// maxBody bounds what Decode and the client read. The served file is about
// 170 KB; 8 MiB leaves room to grow and stops a hostile endpoint.
const maxBody = 8 << 20

// Decode parses and validates a catalog body.
func Decode(data []byte) (*Catalog, error) {
	if len(data) > maxBody {
		return nil, invalid("body is larger than %d bytes", maxBody)
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, invalid("not a JSON catalog: %v", err)
	}
	if c.SchemaVersion != SchemaVersion {
		return nil, invalid("schema_version is %d, this CLI reads %d", c.SchemaVersion, SchemaVersion)
	}
	if len(c.Plugins) == 0 {
		return nil, invalid("it lists no plugins")
	}
	seen := make(map[string]bool, len(c.Plugins))
	for i, p := range c.Plugins {
		if p.Slug == "" {
			return nil, invalid("plugins[%d] has no slug", i)
		}
		if p.Tier != TierFree && p.Tier != TierLicensed {
			return nil, invalid("plugin %q has tier %q, want free or licensed", p.Slug, p.Tier)
		}
		key := p.Slug + "\x00" + p.Tier
		if seen[key] {
			return nil, invalid("plugin %q appears twice at tier %s", p.Slug, p.Tier)
		}
		seen[key] = true
	}
	return &c, nil
}

func invalid(format string, args ...any) error {
	return errs.Newf("E116", "plugin catalog is invalid: %s", fmt.Sprintf(format, args...))
}
