"""Check the proxy legs selected for PRs and manual dispatches."""
import pathlib
import re
import unittest


class WorkflowTest(unittest.TestCase):
    def test_legs(self):
        root = pathlib.Path(__file__).resolve().parents[2]
        workflow = (root / ".github/workflows/proxy-swap-e2e.yml").read_text()
        conditions = dict(re.findall(r"- name: (.+ leg)\n\s+if: \$\{\{ (.+) \}\}", workflow))
        for event, leg, expected in [
            ("pull_request", "", set(conditions)),
            ("workflow_dispatch", "add", {"Add leg"}),
            ("workflow_dispatch", "round-trip", {"Round-trip leg", "Kill-mid-remove leg"}),
            ("workflow_dispatch", "renew", {"Renew leg"}),
        ]:
            with self.subTest(event=event, leg=leg):
                selected = set()
                for name, condition in conditions.items():
                    expression = condition.replace("github.event_name", repr(event))
                    expression = expression.replace("inputs.leg", repr(leg))
                    expression = expression.replace("&&", "and").replace("||", "or")
                    if eval(expression, {"__builtins__": {}}):
                        selected.add(name)
                self.assertEqual(selected, expected)


if __name__ == "__main__":
    unittest.main()
