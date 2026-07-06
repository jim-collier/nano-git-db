# Donation address signing

`--donate` and the TUI/web Donate screens show donation addresses from `source/internal/core/donate/donate.go`. Anyone can edit that file, so the table is signed: the test gate fails unless it carries a valid signature made with the maintainer's key. A changed address that is not re-signed fails the gate, so a stray or malicious edit cannot quietly redirect donations in a release.

## What lives where

- **Private signing key** - `../private/donation_keys/donation_ed25519`, outside the repo (that tree is cloud-synced, so the key is passphrase-protected and only ever stored encrypted). Never commit it.
- **Trust anchor** - `../private/donation_keys/allowed_signers`, also outside the repo. The gate reads the public key from here, not from the repo, so a pull request cannot swap the key along with an address.
- **Signature** - `source/internal/core/donate/donation.sig`, committed next to `donate.go`.
- **Signed content** - `donate.CanonicalBytes()`: the label, kind and value of every entry, in order. Reordering or editing the table changes these bytes and invalidates the signature.

## Generate the key (one time)

```bash
mkdir -p ../private/donation_keys && chmod 700 ../private/donation_keys
ssh-keygen -t ed25519 -C "nano-git-db donation signing" -f ../private/donation_keys/donation_ed25519
printf 'donation namespaces="donation" %s\n' "$(cat ../private/donation_keys/donation_ed25519.pub)" > ../private/donation_keys/allowed_signers
```

Use a strong passphrase (the folder is cloud-synced), and back it up - without it the addresses can never be updated again.

## Set addresses and sign

1. Replace the `PLACEHOLDER_*` values in `source/internal/core/donate/donate.go` with the real addresses/URLs.
2. Sign: `cicd/sign-donations.bash` (prompts for the passphrase; writes `donation.sig`).
3. Commit `donate.go` and `donation.sig` together.

## How the gate works

`internal/core/donate` has a `go test` (`TestDonationTableSigned`) that runs in the gating cicd test stage. It:

- skips while every address is still a placeholder (nothing to protect yet);
- skips when `ssh-keygen` or the trust anchor is absent (e.g. a fresh clone without the key dir), so it never breaks a contributor's test run;
- otherwise verifies `donation.sig` over the current table against the anchor, and fails if it does not match.

Point it at a different anchor with `NGDB_DONATION_ALLOWED_SIGNERS` if needed.

## Rotate / update

Editing an address is the same loop: change `donate.go`, re-run `cicd/sign-donations.bash`, commit both. Replacing the key means generating a new one, re-running the `allowed_signers` line above, then re-signing.
