#!/usr/bin/env bash
# Fixture for scripts/drill/two-command.sh --self-test: a command that asks a
# question. Leg A (stdin /dev/null) gets EOF and fails; leg B (a pseudo-TTY that
# never receives input) blocks until the drill's timeout. Either way the drill
# must flag it.
printf 'Continue? [y/N] '
read -r x || exit 1
printf 'answer: %s\n' "$x"
