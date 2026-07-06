#!/usr/bin/env bash
# Sign the donation address table so the build can detect tampering. The signature
# covers donate.CanonicalBytes() - the label, kind and value of every entry in
# source/donate/donate.go, in order - and the go-test gate re-checks it
# against the out-of-repo trust anchor. Run this after editing the addresses; only
# the private-key holder can produce a signature that verifies, so a changed
# address that was not re-signed fails the gate. See cicd/donation-signing.md.
set -euo pipefail
cd "$(dirname "$0")/.."  ## repo root; this script lives in cicd/

# Key lives outside the repo (passphrase-protected; ssh-keygen prompts for it).
key="${DONATION_SIGNING_KEY:-../private/donation_keys/donation_ed25519}"
sig="source/donate/donation.sig"
namespace="donation"

[[ -f "$key" ]] || {
	echo "Signing key not found: $key" >&2
	echo "Generate it (see cicd/donation-signing.md) or point DONATION_SIGNING_KEY at it." >&2
	exit 1
}

# Canonical bytes come from the app itself, so signer and verifier never disagree.
canon="$(mktemp)"
trap 'rm -f "$canon" "$canon.sig"' EXIT
( cd source && go run -mod=vendor ./cmd/donate-canonical ) > "$canon"

ssh-keygen -Y sign -f "$key" -n "$namespace" "$canon"
mv -f "$canon.sig" "$sig"
echo "signed donation table -> $sig"
