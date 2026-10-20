"""Exercise the kill verdict without Docker or a live build."""
import pathlib
import subprocess
import unittest


class KillVerdictTest(unittest.TestCase):
    def test_verdict(self):
        source = pathlib.Path(__file__).with_name("proxy-swap.sh").read_text()
        block = source.split("    alive=no\n", 1)[1].split("    # Recovery re-runs", 1)[0]
        for alive, killed, status, gone, succeeds in [
            (0, 0, 137, 1, True),
            (1, 1, 0, 1, False),
            (0, 1, 0, 1, False),
            (0, 0, 0, 1, False),
            (0, 0, 137, 0, False),
        ]:
            with self.subTest(alive=alive, killed=killed, status=status, gone=gone):
                prelude = f"""
set -euo pipefail
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
mkdir "$W/evidence"
build_pid=123
gone={gone}
kill() {{ if [[ $1 == -0 ]]; then return {alive}; else return {killed}; fi; }}
wait() {{ return {status}; }}
alive=no
"""
                result = subprocess.run(["bash", "-c", prelude + block], capture_output=True)
                self.assertEqual(result.returncode == 0, succeeds, result.stderr.decode())


if __name__ == "__main__":
    unittest.main()
