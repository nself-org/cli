// Property-based tests: CLI port number validation
// Valid: 1-65535. Invalid: 0, >65535, floats, strings
import fc from 'fast-check';
import { spawnSync } from 'child_process';
import { describe, test } from 'node:test';
import assert from 'node:assert';

const VALIDATOR = `${process.cwd()}/src/lib/utils/validate.sh`;

function validatePort(port) {
  const result = spawnSync('bash', [VALIDATOR, 'validate_port', String(port)], {
    timeout: 1000,
    stdio: 'pipe',
  });
  return result.status === 0;
}

describe('Port validation — property-based', () => {
  test('ports 1-65535 always valid', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 65535 }),
        (port) => validatePort(String(port)) === true
      ),
      { numRuns: 200 }
    );
  });

  test('port 0 always invalid', () => {
    assert.strictEqual(validatePort('0'), false);
  });

  test('ports above 65535 always invalid', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 65536, max: 99999 }),
        (port) => validatePort(String(port)) === false
      ),
      { numRuns: 100 }
    );
  });

  test('non-numeric strings always invalid', () => {
    fc.assert(
      fc.property(
        fc.string().filter(s => !/^\d+$/.test(s) && s.length > 0),
        (port) => validatePort(port) === false
      ),
      { numRuns: 100 }
    );
  });
});
