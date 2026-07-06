// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/jim-collier/nano-git-db/gate"
)

// applyGate configures the server from the startup posture. Nag sets a banner
// and runs normally; Blocking holds every page on a start screen until the user
// waits out the countdown (full read/write) or dismisses it (read-only). Full
// does nothing - the open-source build, with no provider, is always Full.
func (s *server) applyGate(st gate.Status) {
	switch st.Posture {
	case gate.Nag:
		s.banner = st.Message
	case gate.Blocking:
		s.gated = true
		s.gateMsg = st.Message
		s.unlockAt = s.now().Add(time.Duration(st.WaitSeconds) * time.Second)
	}
}

// gateGuard holds the whole UI on the start screen while gated. Assets and the
// /gate/ routes pass through so the screen can render and act.
func (s *server) gateGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.gated ||
			strings.HasPrefix(r.URL.Path, "/assets/") ||
			strings.HasPrefix(r.URL.Path, "/gate/") {
			next.ServeHTTP(w, r)
			return
		}
		s.gatePage(w)
	})
}

// remainingSeconds is how long read/write is still withheld, floored at zero.
func (s *server) remainingSeconds() int {
	remaining := int(s.unlockAt.Sub(s.now()).Seconds())
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *server) gatePage(w http.ResponseWriter) {
	s.render(w, "gate.html", map[string]any{
		"Message":   s.gateMsg,
		"Remaining": s.remainingSeconds(),
	})
}

// gateContinue clears the gate into full read/write, but only once the wait has
// elapsed; pressed early it just redraws the start screen with the time left.
func (s *server) gateContinue(w http.ResponseWriter, r *http.Request) {
	if s.now().Before(s.unlockAt) {
		s.gatePage(w)
		return
	}
	s.gated = false
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// gateDismiss clears the gate into a read-only session.
func (s *server) gateDismiss(w http.ResponseWriter, r *http.Request) {
	s.gated = false
	s.api.SetReadOnly(true)
	s.banner = "read-only - " + s.gateMsg
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
