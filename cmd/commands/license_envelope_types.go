package commands

// Purpose: the typed v1 envelope data of the licence commands (P7-SURF-16).
// Inputs: facts the command bodies already hold. Outputs: Go types registered
// in registry_types_account.go; tools/schemagen turns them into
// schemas/commands/license-*.v1.schema.json.
// Constraints: a licence key reaches a document only through license.MaskKey
// (first 10 and last 4 characters); a JWT, a signature or a raw cache body
// never does. Members are never nil slices so a document prints [] not null.
// SPORT: P7-SURF-16, contract:cli.json-envelope

// licenseKeyRef is one configured key as a document shows it.
type licenseKeyRef struct {
	// Key is the masked key (license.MaskKey).
	Key string `json:"key"`
	// Product is the product name the key prefix selects, "Unknown" for none.
	Product string `json:"product"`
}

// licenseSetData is the data of `license set`.
type licenseSetData struct {
	// Key is the masked key that was saved.
	Key string `json:"key"`
	// Product is the product the key unlocks, "" when the prefix is unknown.
	Product string `json:"product"`
	// Replaced is the number of keys the new key replaced.
	Replaced int `json:"replaced"`
}

// licenseAddData is the data of `license add`.
type licenseAddData struct {
	// Added lists the keys that were validated and saved, in argument order.
	Added []licenseKeyRef `json:"added"`
}

// licenseRemoveData is the data of `license remove`.
type licenseRemoveData struct {
	// Removed is the number of keys that matched and were removed.
	Removed int `json:"removed"`
}

// licenseStatusKey is one row of the status table.
type licenseStatusKey struct {
	// Key is the masked key.
	Key string `json:"key"`
	// Product is the tier or product name.
	Product string `json:"product"`
	// Status is Active, Invalid, Error, Unknown or a rate-limit text.
	Status string `json:"status"`
	// Expires is the expiry the server reported, "-" when none.
	Expires string `json:"expires"`
	// Plugins are the plugins the key covers.
	Plugins []string `json:"plugins"`
}

// licensePluginAccess is the answer to `--plugin <name>`.
type licensePluginAccess struct {
	// Plugin is the plugin that was checked.
	Plugin string `json:"plugin"`
	// Granted is true when an active key covers the plugin.
	Granted bool `json:"granted"`
}

// licenseStatusData is the data of `license status` and `license list`.
type licenseStatusData struct {
	// Keys has one row per configured key; empty when none is set.
	Keys []licenseStatusKey `json:"keys"`
	// Products are the distinct product names, in key order.
	Products []string `json:"products"`
	// Plugins are the distinct plugins the active keys cover, sorted.
	Plugins []string `json:"plugins"`
	// PluginAccess is present only with --plugin.
	PluginAccess *licensePluginAccess `json:"plugin_access,omitempty"`
}

// licenseShowData is the data of `license show`: the pre-contract document
// plus Configured, which is false (and the rest empty) when no key is set.
type licenseShowData struct {
	// Configured is false when no licence key is configured.
	Configured bool `json:"configured"`
	// KeyPrefix is "..." and the last four characters of the first key.
	KeyPrefix string `json:"key_prefix"`
	// Tier is the tier name the key prefix selects.
	Tier string `json:"tier"`
	// Bundles are the bundles the key unlocks.
	Bundles []string `json:"bundles_unlocked"`
	// PluginsCount is the number of plugins the cached validation allows.
	PluginsCount int `json:"plugins_available_count"`
	// DaysToExpire is the days left on the cache, absent without a cache.
	DaysToExpire *int `json:"days_to_expire,omitempty"`
	// CacheExpiry is the cache expiry date or "expired", absent without a cache.
	CacheExpiry string `json:"cache_expiry,omitempty"`
}

// licenseValidateData is the data of `license validate`. A failing key makes
// the command fail, so a document lists keys the server accepted.
type licenseValidateData struct {
	// Keys are the masked keys that validated.
	Keys []string `json:"keys"`
}

// licenseUpgradeData is the data of `license upgrade`.
type licenseUpgradeData struct {
	// URL is the pricing page.
	URL string `json:"url"`
	// Opened is always false in JSON mode: no browser is started.
	Opened bool `json:"opened"`
}

// licenseRevalidateData is the data of `license revalidate`.
type licenseRevalidateData struct {
	// Tier is the tier the server reported.
	Tier string `json:"tier"`
	// ExpiresAt is the expiry date (YYYY-MM-DD), absent when none.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// licenseRevokeData is the data of `license revoke`.
type licenseRevokeData struct {
	// Revoked is false when --yes was missing and nothing changed.
	Revoked bool `json:"revoked"`
}

// licenseRestoreData is the data of `license restore`.
type licenseRestoreData struct {
	// Restored is true once the new key is stored.
	Restored bool `json:"restored"`
	// Product is the product of the new key, "" when the prefix is unknown.
	Product string `json:"product"`
}

// licenseMigrateKey is one row of the migration status check.
type licenseMigrateKey struct {
	// Key is the masked key.
	Key string `json:"key"`
	// Product is the product the prefix selects.
	Product string `json:"product"`
	// Remaining is the grace text ("12 days", "-", "(new format)", "?").
	Remaining string `json:"remaining"`
	// Status is the migration status or the error text.
	Status string `json:"status"`
}

// licenseMigrateLink is the outcome of linking a key to an account.
type licenseMigrateLink struct {
	// Key is the masked key.
	Key string `json:"key"`
	// AccountID is the account the key is linked to.
	AccountID string `json:"account_id"`
	// Tier is the tier the server reported.
	Tier string `json:"tier"`
	// AlreadyLinked is true when the key was linked before this run.
	AlreadyLinked bool `json:"already_linked"`
}

// licenseMigrateData is the data of `license migrate`.
type licenseMigrateData struct {
	// Mode is "status" (no --account-id) or "link".
	Mode string `json:"mode"`
	// Keys is the status table; empty in link mode.
	Keys []licenseMigrateKey `json:"keys"`
	// Link is present in link mode.
	Link *licenseMigrateLink `json:"link,omitempty"`
}

// licenseRefreshKey is the refresh outcome of one key.
type licenseRefreshKey struct {
	// Key is the masked key.
	Key string `json:"key"`
	// Refreshed is true when the server confirmed the key.
	Refreshed bool `json:"refreshed"`
	// Tier is the tier the server reported, absent on failure.
	Tier string `json:"tier,omitempty"`
	// ExpiresAt is the expiry date (YYYY-MM-DD), absent on failure.
	ExpiresAt string `json:"expires_at,omitempty"`
	// Error is why the key was not refreshed, absent on success.
	Error string `json:"error,omitempty"`
}

// licenseRefreshData is the data of `license refresh`.
type licenseRefreshData struct {
	// Mode is keys, scheduled, remove-timer or timer-preview.
	Mode string `json:"mode"`
	// Keys has one row per key in keys mode; empty otherwise.
	Keys []licenseRefreshKey `json:"keys"`
	// EntitlementExpires is the new entitlement expiry (RFC 3339) in scheduled mode.
	EntitlementExpires string `json:"entitlement_expires,omitempty"`
	// TimerRemoved is true when remove-timer or a scheduled run removed a timer.
	TimerRemoved bool `json:"timer_removed,omitempty"`
	// Preview is the unit text timer-preview would install.
	Preview string `json:"preview,omitempty"`
}

// licenseExportData is the data of `license export`. The signed cache itself
// holds the licence JWT, so a document carries its summary and never the blob.
type licenseExportData struct {
	// Tier is the tier of the cached validation.
	Tier string `json:"tier"`
	// PluginsAllowed are the plugins the cached validation allows.
	PluginsAllowed []string `json:"plugins_allowed"`
	// FetchedAt is the Unix time the cache was fetched.
	FetchedAt int64 `json:"fetched_at"`
	// ExpiresAt is the Unix time the cache expires.
	ExpiresAt int64 `json:"expires_at"`
	// Signed is true when the cache signature verifies.
	Signed bool `json:"signed"`
}

// licenseSimulateData is the data of `license simulate-offline`.
type licenseSimulateData struct {
	// Cleared is true after --clear; the other members are then zero.
	Cleared bool `json:"cleared"`
	// Days is the number of simulated offline days.
	Days int `json:"days"`
	// State is the grace state the simulation reached.
	State string `json:"state"`
	// CanProceed reports whether commands may still run.
	CanProceed bool `json:"can_proceed"`
	// WriteAllowed reports whether writes are still allowed.
	WriteAllowed bool `json:"write_allowed"`
	// Message is the grace message, absent when none.
	Message string `json:"message,omitempty"`
	// Banner is the warning banner text, absent when none.
	Banner string `json:"banner,omitempty"`
}
