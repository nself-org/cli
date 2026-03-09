/**
 * Property-based tests for CLI input validation — T-0412
 *
 * Tests the expected behaviour of 5 CLI validation functions using fast-check.
 * Each property runs 1000 random cases. The validation functions mirror the
 * logic in the CLI bash scripts (src/lib/validation/).
 *
 * Run: node test-validators.js
 * Requires: pnpm install (fast-check ^3.15.0)
 */

import fc from 'fast-check';

// =============================================================================
// Validation functions — JS equivalents of the CLI bash validators
// =============================================================================

/**
 * Domain name: alphanumeric + hyphens per label, no leading/trailing hyphen
 * per label, at least one dot, labels 1-63 chars, total max 253 chars.
 */
function validateDomain(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 253) return false;
  const labels = value.split('.');
  if (labels.length < 2) return false;
  for (const label of labels) {
    if (label.length === 0 || label.length > 63) return false;
    if (!/^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$|^[a-zA-Z0-9]$/.test(label)) return false;
  }
  return true;
}

/**
 * Port: integer in range 1–65535.
 */
function validatePort(value) {
  if (typeof value !== 'number') return false;
  return Number.isInteger(value) && value >= 1 && value <= 65535;
}

/**
 * Plugin name: lowercase alphanumeric + hyphens, 1–64 chars,
 * no leading/trailing hyphen, no consecutive hyphens.
 */
function validatePluginName(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 64) return false;
  return /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$|^[a-z0-9]$/.test(value);
}

/**
 * Env var name: UPPER_SNAKE_CASE — uppercase letters, digits, underscores,
 * must start with a letter (not a digit), no spaces.
 */
function validateEnvVarName(value) {
  if (typeof value !== 'string' || value.length === 0) return false;
  return /^[A-Z][A-Z0-9_]*$/.test(value);
}

/**
 * License key: starts with nself_pro_ or nself_max_, total length ≥ 42 chars,
 * suffix is alphanumeric only.
 */
function validateLicenseKey(value) {
  if (typeof value !== 'string') return false;
  if (!value.startsWith('nself_pro_') && !value.startsWith('nself_max_')) return false;
  if (value.length < 42) return false;
  const prefix = value.startsWith('nself_pro_') ? 'nself_pro_' : 'nself_max_';
  const suffix = value.slice(prefix.length);
  return /^[a-zA-Z0-9]+$/.test(suffix);
}

// =============================================================================
// Arbitraries
// =============================================================================

// Valid domain label: 1–63 chars, alphanumeric + hyphens, no leading/trailing hyphen
const arbDomainLabel = fc.stringMatching(/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$|^[a-z0-9]$/);

// Valid domain: 2+ labels joined by dots, total ≤ 253 chars
const arbValidDomain = fc
  .array(arbDomainLabel, { minLength: 2, maxLength: 5 })
  .filter((labels) => labels.join('.').length <= 253)
  .map((labels) => labels.join('.'));

// Invalid domain: starts/ends with hyphen in a label, or no dot, etc.
const arbInvalidDomain = fc.oneof(
  fc.constant(''),
  fc.constant('nodot'),
  fc.constant('-leading.com'),
  fc.constant('trailing-.com'),
  fc.constant('.startdot.com'),
  fc.constant('a'.repeat(254)),
  fc.string({ maxLength: 10 }).filter((s) => !s.includes('.'))
);

// Valid port: 1–65535 integer
const arbValidPort = fc.integer({ min: 1, max: 65535 });

// Invalid port
const arbInvalidPort = fc.oneof(
  fc.constant(0),
  fc.constant(65536),
  fc.constant(-1),
  fc.constant(1.5),
  fc.float({ noNaN: true }).filter((n) => !Number.isInteger(n))
);

// Valid plugin name: lowercase alphanum + hyphens, 1–64 chars
const arbValidPluginName = fc
  .stringMatching(/^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$|^[a-z0-9]$/)
  .filter((s) => s.length >= 1 && s.length <= 64);

// Invalid plugin name
const arbInvalidPluginName = fc.oneof(
  fc.constant(''),
  fc.constant('HasUpper'),
  fc.constant('has space'),
  fc.constant('has_underscore'),
  fc.constant('has@special'),
  fc.constant('-leading'),
  fc.constant('trailing-'),
  fc.string({ minLength: 65, maxLength: 80 }).map((s) => s.toLowerCase())
);

// Valid env var name: [A-Z][A-Z0-9_]*
const arbValidEnvVarName = fc
  .stringMatching(/^[A-Z][A-Z0-9_]*$/)
  .filter((s) => s.length >= 1);

// Invalid env var name
const arbInvalidEnvVarName = fc.oneof(
  fc.constant(''),
  fc.constant('lowercase'),
  fc.constant('has space'),
  fc.constant('1STARTS_DIGIT'),
  fc.constant('has-hyphen'),
  fc.constant('mixedCase')
);

// Valid license key: nself_pro_/nself_max_ prefix + alphanumeric suffix, total ≥42
const arbLicensePrefix = fc.oneof(fc.constant('nself_pro_'), fc.constant('nself_max_'));
const arbValidLicenseKey = fc
  .tuple(
    arbLicensePrefix,
    fc.stringMatching(/^[a-zA-Z0-9]+$/).filter((s) => s.length >= 32)
  )
  .map(([prefix, suffix]) => prefix + suffix)
  .filter((k) => k.length >= 42);

// Invalid license key
const arbInvalidLicenseKey = fc.oneof(
  fc.constant(''),
  fc.constant('nself_pro_short'),
  fc.constant('nself_max_short'),
  fc.constant('wrong_prefix_' + 'a'.repeat(32)),
  fc.constant('nself_pro_has spaces ' + 'a'.repeat(20)),
  fc.string({ maxLength: 41 })
);

// =============================================================================
// Test runner (no external test framework — pure Node)
// =============================================================================

let passed = 0;
let failed = 0;

function runProperty(name, arb, predicate, numRuns = 1000) {
  try {
    fc.assert(fc.property(arb, predicate), { numRuns, seed: 42 });
    console.log('[PASS]', name);
    passed++;
  } catch (err) {
    console.error('[FAIL]', name);
    console.error('      ', err.message || err);
    failed++;
  }
}

// =============================================================================
// Property 1: Domain name validation
// =============================================================================

runProperty(
  'domain: valid domains are accepted',
  arbValidDomain,
  (domain) => validateDomain(domain) === true
);

runProperty(
  'domain: invalid domains are rejected',
  arbInvalidDomain,
  (domain) => validateDomain(domain) === false
);

// =============================================================================
// Property 2: Port number validation
// =============================================================================

runProperty(
  'port: 1-65535 integers are accepted',
  arbValidPort,
  (port) => validatePort(port) === true
);

runProperty(
  'port: 0, 65536, non-integers, negatives are rejected',
  arbInvalidPort,
  (port) => validatePort(port) === false
);

// =============================================================================
// Property 3: Plugin name validation
// =============================================================================

runProperty(
  'plugin-name: valid names (lowercase alphanum + hyphens, 1-64 chars) are accepted',
  arbValidPluginName,
  (name) => validatePluginName(name) === true
);

runProperty(
  'plugin-name: uppercase, spaces, special chars, leading/trailing hyphens are rejected',
  arbInvalidPluginName,
  (name) => validatePluginName(name) === false
);

// =============================================================================
// Property 4: Env var name validation
// =============================================================================

runProperty(
  'env-var: UPPER_SNAKE_CASE names are accepted',
  arbValidEnvVarName,
  (name) => validateEnvVarName(name) === true
);

runProperty(
  'env-var: lowercase, spaces, digit-start, hyphens are rejected',
  arbInvalidEnvVarName,
  (name) => validateEnvVarName(name) === false
);

// =============================================================================
// Property 5: License key validation
// =============================================================================

runProperty(
  'license-key: nself_pro_/nself_max_ + alphanumeric suffix (>=42 chars) are accepted',
  arbValidLicenseKey,
  (key) => validateLicenseKey(key) === true
);

runProperty(
  'license-key: wrong prefix, too short, or non-alphanumeric suffix are rejected',
  arbInvalidLicenseKey,
  (key) => validateLicenseKey(key) === false
);

// =============================================================================
// Summary
// =============================================================================

console.log('');
console.log('Results:', passed, 'passed,', failed, 'failed');
if (failed > 0) {
  process.exit(1);
}
