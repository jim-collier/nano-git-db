// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jim-collier/nano-git-db/donate"
)

// Donate prints the project's donation targets. No database is involved.
func Donate() error {
	fmt.Println(donate.Intro)
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, t := range donate.Targets {
		value := t.Value
		if !t.Configured() {
			value = "(not yet configured)"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", t.Label, value)
	}
	return tw.Flush()
}
