<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Research: enterprise license validation scheme

Proposal for the backlog "Enterprise license validation scheme" item. Draft to react to. Companion: [research-enterprise-concerns.md](research-enterprise-concerns.md), which covers where the pieces live and the signing-key boundary.

## Requirements, distilled

From the backlog, in the owner's words: phones home to verify a subscription is active; allows N copies at once; does not fail if it cannot phone home for a while; does not bind to specific hardware.

Read carefully, those four lines already pick the design. "Does not fail when offline" plus "no hardware binding" rules out the strict always-online or fingerprint-locked schemes. What is left is the well-worn pattern: a signed token the product can verify entirely offline, refreshed by an occasional check-in, with soft seat counting.

## The shape

Two moving parts, one embedded constant.

- Embedded constant: the enterprise build ships with the vendor's public verification key baked in. It can verify a license; it can never mint one (the private key lives only on the vendor side - see the companion doc).
- License token: a small signed blob the vendor issues at purchase and re-issues on each successful check-in. The product verifies it offline against the embedded public key. Its validity window is what makes offline tolerance automatic - the token is proof of entitlement until it expires, no network required.
- Phone-home: a periodic check-in to the vendor's validation endpoint that confirms the subscription is still active (not canceled or refunded), hands back a fresh token with a pushed-forward window, and updates soft seat accounting. This is a refresh, not a gate: a failed check-in changes nothing until the current token's window actually runs out.

## The token

Keep it tiny and offline-verifiable. Proposed fields:

- customer id
- plan / tier
- seat count N
- issued-at
- expires-at (the window edge - see grace below)
- instance-agnostic; no machine identifiers
- signature over all of the above

Signature: Ed25519 (`crypto/ed25519`, stdlib - no new dependency, small keys, fast verify). The token itself can be a compact custom encoding (the fields plus a base64 signature) rather than JWT, which avoids pulling a JWT library and keeps the format under the project's own control. This mirrors the encryption work's stdlib-only stance.

Because verification is a stdlib signature check against a baked-in public key, the product validates a license with zero network and zero dependencies.

## Grace, by construction

There is no separate "grace timer" to build - the token's window is the grace period. Issue tokens with a window comfortably longer than the check-in interval (illustrative: check in weekly, issue a 30-45 day window). As long as check-ins succeed, the window keeps sliding forward and the customer never notices. A network outage, a locked-down site, a long offline stretch - all invisible until the window edge is actually reached.

Only when the token is past expires-at and every check-in attempt across the whole window has failed does the product treat the license as lapsed. That is the sole failure point, and it is weeks after the last successful contact.

## Failure mode when the window finally lapses

This is a policy decision, not a technical one, and it wants the owner's call. Recommendation: never punish with data loss or a hard lock. A ladder:

- Window healthy: full function, silent.
- Past expires-at, still trying: a non-blocking banner ("could not reach licensing - will keep trying"), full function continues. This covers real outages.
- Long past the window with sustained failure: degrade to read-only, still no data loss, with a clear message and a manual re-activate path. The user's data is theirs; the product going read-only protects the subscription without holding data hostage.

Hard-blocking or corrupting on lapse is off the table - it would be a support disaster for exactly the honest customer hit by an outage.

## Seats without hardware binding

"N copies at once, no hardware binding" makes seat enforcement inherently soft, and that is fine. Each install mints a random instance id (a UUID stored per install, not derived from MAC/CPU/disk). Check-ins carry that id plus a heartbeat; the validation server counts active instances by id with a TTL, so an install that stops checking in ages out of the count.

Over-count behavior is a decision:

- Soft (recommended): allow the over-count, surface it to the account owner ("6 of 5 seats active"), and let the sale be the enforcement. Friendly, and it matches the "no hardware binding" spirit.
- Firmer: refuse to issue a fresh token to a new instance once N are active, so existing installs keep working but a genuinely new one cannot activate until a seat frees up. Never revokes an already-running instance.

Either way, an already-licensed running instance is never yanked mid-session.

## Endpoint surface

Small enough for stdlib `net/http`, no framework (see the companion doc's minimal-dependencies argument):

- check-in / refresh: product sends customer id + instance id + current token; server verifies the subscription is active, returns a freshly signed token (window pushed forward) or a "subscription inactive" that the product surfaces without immediately locking (the current window still governs).
- optionally, deactivate: product tells the server an instance is retiring, freeing a seat immediately rather than waiting for TTL.

Everything the endpoint returns is signed, so a tampering proxy cannot forge a longer window.

## What stays out of the open-source build

All of this is enterprise-only. The public AGPL build has no license check, no phone-home, no embedded key - it must not, both on principle (AGPL) and because a license gate in an open build is trivially patched out and pointless. This lives entirely on the `github_private` product side, talking to the vendor side.

## Open decisions for you

- Check-in interval and token window length (recommend interval well inside the window; illustrative weekly / 30-45 days).
- Lapse failure mode (recommend the banner -> read-only ladder, never data loss / hard lock).
- Over-seat behavior (recommend soft with owner notification).
- Token encoding: custom compact blob (recommended, no JWT dep) vs JWT.
- Whether "deactivate to free a seat" is worth building in v1 or left to TTL aging.
- Where the entitlement/seat datastore lives (tie-in to the companion doc's datastore question).
