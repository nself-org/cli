#!/usr/bin/env bash
# nohup stub: run the command in the foreground of its own background job and
# record the exit code, so the test can assert the detached watchdog's status.
"$@"
echo $? > "${STUB_DIR}/watchdog.rc"
