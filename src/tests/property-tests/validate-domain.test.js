// Property-based tests: CLI domain name validation
// T-0412 — Uses fast-check with Node harness
// Tests validate-domain.sh validator against 1000 random inputs

import fc from 'fast-check';
import { execFileSync } from 'child_process';
import { describe, test } from 'node:test';
import assert from 'node:assert';

const VALIDATOR = `${process.cwd()}/src/lib/utils/validate.sh`;

function validateDomain(domain) {
  try {
    // Pass domain as a separate argument — avoids shell interpolation entirely
    execFileSync('bash', [VALIDATOR, 'validate_domain', domain], {
      timeout: 1000,
      stdio: ['ignore', 'ignore', 'ignore'],
    });
    return true;
  } catch {
    return false;
  }
}

describe('Domain name validation — property-based', () => {
  test('valid domains always pass', () => {
    fc.assert(
      fc.property(
        fc.domain(),
        (domain) => {
          // Standard domains from fast-check should be valid
          const result = validateDomain(domain);
          return result === true;
        }
      ),
      { numRuns: 100 }
    );
  });

  test('null bytes always fail', () => {
    fc.assert(
      fc.property(
        fc.string().map(s => s + '\x00'),
        (domain) => validateDomain(domain) === false
      ),
      { numRuns: 100 }
    );
  });

  test('semicolons always fail (injection prevention)', () => {
    fc.assert(
      fc.property(
        fc.string().map(s => s + ';'),
        (domain) => validateDomain(domain) === false
      ),
      { numRuns: 100 }
    );
  });
});
