package errs

// Block is one allocated range of error-code numbers and the single category
// every code in it carries (D13 of the P7-REG Epic, the canonical allocation).
//
// Purpose: the category of a code is a function of its number; Register fills
// an empty Category from this table and rejects a mismatch.
//
// Constraints: a new block needs a line in the Epic allocation table first;
// blocks never overlap and never reach the reserved external block.
type Block struct {
	Lo, Hi   int
	Category string
}

// Owner is a sub-range of a block assigned to one Ticket and the one fragment
// file that may register codes in it.
//
// Purpose: block ownership as data. Register records a problem for a code
// that lies in no Owner range (spare or free numbers fail until the lead adds
// a line to the Epic table and here) or that is registered from a fragment
// other than the owner's. A test proves the ranges are disjoint and nested
// inside one block.
type Owner struct {
	Lo, Hi   int
	Who      string // Ticket id, e.g. "P7-PLUG-01"
	Fragment string // base name of the fragment file, e.g. "codes_plugin_manifest.go"
}

// Reserved external block: the ci plugin keeps its own registry here
// (plugins:free/ci/internal/model). A cli fragment in this range is a
// registry problem.
const (
	reservedLo = 600
	reservedHi = 719
)

// Blocks is the ordered block table.
var Blocks = []Block{
	{1, 49, "docker"},
	{50, 99, "config"},
	{100, 149, "plugin"},
	{150, 199, "ssl"},
	{200, 249, "database"},
	{250, 299, "health"},
	{300, 349, "init"},
	{350, 399, "domain"},
	{400, 449, "cli"},
	{450, 479, "reconcile"},
	{480, 499, "deploy"},
	{500, 529, "adopt"},
	{540, 549, "secret"},
}

// Owners lists the per-Ticket sub-ranges fixed by each owner Epic (D13), with
// the fragment each registers from. Spare and free numbers are deliberately
// absent: registering one is a RegistryErrors() problem. Sorted by Lo.
var Owners = []Owner{
	{1, 5, "existing", "codes_docker.go"},
	{50, 56, "existing", "codes_config.go"}, {57, 59, "P7-REG-01", "codes_config.go"},
	{60, 61, "P7-TRUTH-12", "codes_compat.go"},
	{100, 105, "existing", "codes_plugin.go"}, {106, 110, "P7-REG-01", "codes_plugin.go"},
	{111, 114, "P7-PLUG-01", "codes_plugin_manifest.go"},
	{115, 117, "P7-PLUG-09", "codes_catalog.go"},
	{118, 120, "P7-PLUG-59", "codes_plugin_migrate.go"},
	{121, 124, "P7-PLUG-23", "codes_plugin_signature.go"},
	{125, 126, "P7-PLUG-24", "codes_entitlement.go"},
	{127, 127, "P7-PLUG-32", "codes_plugin_seed.go"},
	{128, 131, "P7-PLUG-17", "codes_plugin_fragment.go"},
	{132, 132, "P7-PLUG-64", "codes_entitlement_refresh.go"},
	{133, 134, "P7-PLUG-20", "codes_bundle_remove.go"},
	{135, 136, "P7-PLUG-19", "codes_bundle_install.go"},
	{137, 137, "P7-PLUG-25", "codes_template.go"},
	{138, 138, "P7-PLUG-60", "codes_object_storage.go"},
	{139, 139, "P7-PLUG-31", "codes_update_stack.go"},
	{150, 151, "existing", "codes_ssl.go"},
	{200, 202, "existing", "codes_database.go"}, {203, 216, "P7-REG-01", "codes_database.go"},
	{217, 221, "P7-PROD-07", "codes_backup.go"}, {222, 224, "P7-PROD-08", "codes_backup_key.go"},
	{250, 252, "existing+P7-REG-01", "codes_health.go"},
	{300, 301, "existing", "codes_init.go"}, {350, 351, "existing", "codes_domain.go"},
	{400, 404, "P7-REG-01", "codes_cli.go"},
	{405, 407, "P7-CANON-01", "codes_mount.go"}, {410, 410, "P7-CANON-21", "codes_canon.go"},
	{420, 423, "P7-SURF-24", "codes_invoke.go"}, {424, 426, "P7-SURF-25", "codes_gate.go"},
	{428, 432, "P7-SURF-02", "codes_httpapi.go"}, {433, 433, "P7-SURF-03", "codes_mcpgen.go"},
	{434, 434, "P7-SURF-06", "codes_config_explain.go"}, {435, 436, "P7-SURF-07", "codes_nself_yaml.go"},
	{437, 437, "P7-SURF-30", "codes_registry_surface.go"},
	{450, 454, "P7-LIVE-03", "codes_reconcile.go"}, {455, 459, "P7-LIVE-09", "codes_lock.go"},
	{460, 464, "P7-LIVE-13", "codes_oplock.go"}, {465, 469, "P7-LIVE-18", "codes_images.go"},
	{470, 474, "P7-LIVE-22", "codes_acme.go"},
	{480, 482, "P7-DEPL-01", "codes_proxy.go"}, {483, 483, "P7-DEPL-12", "codes_deploy_env.go"},
	{484, 487, "P7-DEPL-13", "codes_deploy.go"}, {488, 490, "P7-DEPL-15", "codes_promote.go"},
	{491, 491, "P7-DEPL-18", "codes_archguard.go"}, {492, 499, "P7-DEPL-19", "codes_release_verify.go"},
	{500, 502, "P7-ADOPT-01", "codes_custom_service.go"}, {503, 505, "P7-ADOPT-03", "codes_shared_proxy.go"},
	{506, 506, "P7-ADOPT-21", "codes_shared_proxy_runtime.go"}, {507, 508, "P7-ADOPT-06", "codes_pg_requires.go"},
	{509, 512, "P7-ADOPT-22", "codes_pg_image.go"}, {513, 514, "P7-ADOPT-07", "codes_vector_search.go"},
	{515, 516, "P7-ADOPT-23", "codes_portable.go"}, {517, 524, "P7-ADOPT-10", "codes_importer.go"},
	{525, 527, "P7-ADOPT-17", "codes_export.go"}, {528, 528, "P7-ADOPT-01", "codes_custom_service.go"},
	{529, 529, "P7-ADOPT-10", "codes_importer.go"},
	{540, 544, "P7-TRUST-03", "codes_secret_resolve.go"},
}

// ownerFor returns the Owner whose range contains code number n.
func ownerFor(n int) (Owner, bool) {
	for _, o := range Owners {
		if n >= o.Lo && n <= o.Hi {
			return o, true
		}
	}
	return Owner{}, false
}

// blockFor returns the block that contains code number n.
func blockFor(n int) (Block, bool) {
	for _, b := range Blocks {
		if n >= b.Lo && n <= b.Hi {
			return b, true
		}
	}
	return Block{}, false
}
