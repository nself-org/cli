"""Offline regression cases for D-0829; no GitHub calls."""
import pathlib
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


def merge_source():
    script = (ROOT / 'admin-merge.sh').read_text()
    if '# ── Merge PR' in script:
        return script.split('# ── Merge PR', 1)[1].split('\n', 1)[1].split('# ── Wait for CI', 1)[0]
    return (ROOT / 'lib/admin_merge_pr.sh').read_text()


class MergeTests(unittest.TestCase):
    def test_cleanup(self):
        for merge_rc, armed in [(0, 'null'), (0, 'armed'), (1, 'null'), (1, 'armed')]:
            with self.subTest(merge_rc=merge_rc, armed=armed):
                setup = '''
DRY_RUN=0 PR_NUMBER=7 TARGET_REPO=test/repo MERGE_VERIFY_SECONDS=0 MERGE_POLL_SECONDS=1
info() { echo "$*"; }; ok() { echo "$*"; }; error() { echo "$*"; }; audit() { echo "$*"; }
gh() {
 case "$*" in
 *--disable-auto*) echo DISABLE; return 1;;
 *autoMergeRequest*) echo ARMED_VALUE;;
 *--json\\ state*) echo OPEN;;
 *) return MERGE_RC;;
 esac
}
'''.replace('ARMED_VALUE', armed).replace('MERGE_RC', str(merge_rc))
                result = subprocess.run(['/bin/bash', '-c', setup + merge_source()], text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual('Auto-merge was disabled again.' in result.stdout, armed == 'null', result.stdout)
                self.assertEqual('auto_merge_still_armed' in result.stdout, armed != 'null', result.stdout)

    def test_decimal_timeout(self):
        script = (ROOT / 'admin-merge.sh').read_text()
        constants = script.split('# ── Constants', 1)[1].split('\n', 1)[1].split('AUDIT_LOG=', 1)[0]
        for value, expected in [('08', '8'), ('000', '0'), ('09', '9'), ('60', '60')]:
            with self.subTest(value=value):
                result = subprocess.run(['/bin/bash', '-c', constants + '\nprintf "%s" "$((MERGE_VERIFY_SECONDS + 0))"'], env={'MERGE_VERIFY_SECONDS': value}, text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, expected)


if __name__ == '__main__':
    unittest.main()
