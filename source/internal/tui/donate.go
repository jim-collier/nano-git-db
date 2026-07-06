// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package tui

import (
	"fmt"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/donate"
)

// donateModal shows the project's donation targets in a dismissable dialog.
// Unset addresses read as "not yet configured" (and there is nothing to copy).
func (p *picker) donateModal() {
	var b strings.Builder
	b.WriteString(donate.Intro)
	for _, t := range donate.Targets {
		value := t.Value
		if !t.Configured() {
			value = "(not yet configured)"
		}
		fmt.Fprintf(&b, "\n\n%s\n%s", t.Label, value)
	}
	p.modal("donate", b.String(), []string{"Close"}, func(string) {})
}
