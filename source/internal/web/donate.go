// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package web

import (
	"net/http"

	"github.com/jim-collier/nano-git-db/donate"
)

// donate renders the donation targets partial into #main. Unconfigured targets
// show as "not yet configured" and carry no copyable value.
func (s *server) donate(w http.ResponseWriter, r *http.Request) {
	s.render(w, "donate.html", map[string]any{
		"Intro": donate.Intro, "Targets": donate.Targets,
	})
}
