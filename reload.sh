#!/bin/sh
# Rebuilds rulebook.db from the sources. Register responses are cached in
# .local/register, so a re-run only fetches what it hasn't seen.
set -e
go run ./cmd/load-register -titles C2004Q00685 -text
go run ./cmd/load-referendums
