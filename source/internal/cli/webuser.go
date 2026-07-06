// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// webUser sets (or replaces) a proxied-mode web login. The password comes from
// NGDB_WEB_PASSWORD when set - so it can be scripted - else it is read from
// stdin. The creds file lives in the user config dir, outside any synced tree.
func webUser(username string) error {
	password := os.Getenv("NGDB_WEB_PASSWORD")
	if password == "" {
		fmt.Fprint(os.Stderr, "password: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		password = strings.TrimRight(line, "\r\n")
	}
	if password == "" {
		return fmt.Errorf("webuser: empty password")
	}
	creds := config.LoadWebCreds()
	if err := creds.Set(username, password); err != nil {
		return err
	}
	if err := creds.Save(); err != nil {
		return err
	}
	fmt.Printf("web login set for %q; the web UI requires web_mode = proxied\n", username)
	return nil
}
