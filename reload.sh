#!/bin/sh
# Rebuilds rulebook.db from the sources. Register responses are cached in
# .local/register, so a re-run only fetches what it hasn't seen.
set -e
go run ./cmd/load-register -titles C2004Q00685 -text
go run ./cmd/load-referendums
go run ./cmd/load-governments
# Fetches only in the Register's quiet hours (20:00-08:00 AEST) unless
# cached; a first full run takes about 4 hours. See README, "Acts".
go run ./cmd/load-acts
go run ./cmd/derive
