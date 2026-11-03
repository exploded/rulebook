#!/bin/sh
# Rebuilds rulebook.db from the sources. Register responses are cached in
# .local/register, so a re-run only fetches what it hasn't seen.
set -e
go run ./cmd/load-register -titles C2004Q00685 -text
go run ./cmd/load-referendums
go run ./cmd/load-governments
# These fetch only in the Register's quiet hours (20:00-08:00 AEST) unless
# cached. A first full run of load-acts takes about 3 hours; load-text about
# as long again. See README, "Acts" and "Act text".
go run ./cmd/load-acts
go run ./cmd/load-departments
go run ./cmd/load-text
go run ./cmd/derive
