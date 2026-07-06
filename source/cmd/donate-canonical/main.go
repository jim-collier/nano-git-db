// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// donate-canonical writes the donation table's canonical bytes to stdout, so the
// signer (cicd/sign-donations.bash) signs exactly what the verify gate checks.
// Maintainer-only; the release build (cicd/build.bash) builds only ./cmd/ngdb, so
// this never ships.
package main

import (
	"os"

	"github.com/jim-collier/nano-git-db/internal/core/donate"
)

func main() {
	os.Stdout.Write(donate.CanonicalBytes())
}
