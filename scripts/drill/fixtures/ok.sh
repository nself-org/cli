#!/usr/bin/env bash
# Fixture for scripts/drill/two-command.sh --self-test: a command that never
# prompts and exits 0. The detector must report no prompt for it.
printf 'no prompt here\n'
exit 0
