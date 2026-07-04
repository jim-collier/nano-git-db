#!/bin/bash

##	Purpose:
##		- CI/CD-friendly test harness that passes or fails.
##		- Static checks (go vet, gofmt) plus the full Go test suite, run from source/ (the module root).
##		- Smoke-tests the built binary if one is present.
##	History: At bottom of this file. (Note: History for this is maintained outside of [or in addition to] git project.)

##	Copyright © 2026 Jim Collier (ID: 1cv◂‡Vᛦ)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -euo pipefail
cd "$(dirname "$0")/../source"

echo "go vet ..."
go vet -mod=vendor ./...

echo "gofmt check ..."
unformatted="$(gofmt -l cmd internal)"
[[ -n "${unformatted}" ]]  &&  { echo "gofmt needed: ${unformatted}"; exit 1; }

echo "go test ..."
go test -mod=vendor ./...

## Smoke-test the built binary if present. --help prints usage and exits 0.
## A bare invocation would try to open the TUI, which needs a terminal.
exe="../bin/nanogitdb"
if [[ -x "${exe}" ]]; then
	echo "smoke test: ${exe}"
	"${exe}" --help >/dev/null
fi

echo "test.bash: PASS"


##	Script history:
##		- 20260420 JC: Created (convert-base-v2 template).
##		- 20260701 JC: Replaced the template's round-trip suite with this project's Go checks.
##		- 20260701 JC: Binary moved to bin/ (build.bash now lives in cicd/ and outputs there).
