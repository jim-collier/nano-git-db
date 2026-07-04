<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Research: separating enterprise concerns

Proposal for backlog "Misc to-do" (concern separation) and the "folder hierarchy" feature that follows it. This is a draft to react to, not a decision. Companion: [research-license-validation.md](research-license-validation.md).

## The question

Today there are two code trees: the public AGPL build (`github/`) and the proprietary enterprise superset (`github_private`, encryption + REST on top of the same core). The backlog asks a third thing to exist somewhere: the vendor's own side that mints license keys, takes payment, and answers "is this subscription still active?". That is a different kind of thing from a product - it is infrastructure the vendor runs and never ships.

So the real task is naming the concerns, then drawing the boundary in the one place that actually matters: what leaves the building.

## The concerns, named

Five distinct things, grouped by who holds them:

- Shipped to customers:
	- Public product - the AGPL build. Already exists.
	- Enterprise product - proprietary superset (encryption, REST, and later the license check). Runs on the customer's machine or their own server. Already exists in `github_private`.
- Run by the vendor, never shipped:
	- License issuance - generates the signed license token/key. Holds the private signing key.
	- Payment and subscription - the store: checkout, plan, renewal, cancellation, refund. Integrates a payment processor.
	- Validation service - the small web endpoint the enterprise product phones home to. Confirms a subscription is still active, refreshes the token window, and does soft seat accounting.

The last three are one company, not three products. They can live together, but they must live apart from anything a customer receives.

## The one boundary that matters

The private signing key must never exist in a customer artifact. Everything else is organizational preference; this one is a hard security line. A customer who can reach the signing key can mint their own licenses, and no amount of obfuscation fixes that. So the boundary is drawn by asset, not by feature:

- The enterprise product embeds only the vendor's public verification key. It can check a license, never issue one.
- The signing key lives only in the licensing backend, ideally in a secret store / HSM, never in a repo.

Given that, the vendor-run side wants its own repo (or at minimum its own tightly-scoped module with no build path that could pull a secret into a shipped binary). A separate repo makes the mistake structurally hard rather than merely discouraged.

## Structure options

- Option A - three repos (recommended): `github` (public, exists), `github_private` (enterprise product, exists), and a new `github_vendor` (or similarly named) for issuance + store + validation. The signing key lives only in the third, which never produces a customer artifact. Cleanest separation of the asset boundary; the two customer-facing repos physically cannot contain the signing key because they never reference the code that holds it.
	- Cost: a third repo to maintain, and shared types (the license token format) must be duplicated or pulled from a small shared module.
- Option B - vendor side as a second module inside `github_private`: enterprise product and vendor backend share one private repo, separate top-level modules with no cross-import from product to backend. Fewer repos; the boundary becomes a lint/CI rule ("product must not import backend") instead of a physical wall.
	- Cost: one CI misconfiguration and a secret-holding package is one import away from a shipped binary. Weaker on exactly the line that matters most.
- Option C - monorepo, module boundaries only: everything in one tree, separated by directory. Rejected for this project: it puts the signing key's neighborhood in the same tree as customer code, which is the opposite of the goal.

Recommendation: Option A. The extra repo is cheap next to the cost of leaking a signing key.

## The license token format is the shared contract

Whatever the repo layout, one small thing is shared between the vendor side (issues it) and the enterprise product (verifies it): the license token's byte format and the public key. Keep this tiny and stable - a handful of fields and a signature, defined once. See [research-license-validation.md](research-license-validation.md) for the proposed format. If Option A, this can be a small shared Go module vendored into both consumers, or simply a documented spec re-implemented on each side (it is small enough that duplication is defensible and avoids a shared dependency).

## Minimal web dependencies (the stated requirement)

The backlog calls this out for the vendor side specifically, and the reasoning is sound: the store and validation endpoint would otherwise be the project's one place tempted into a large web dependency tree, with the churn and supply-chain exposure that brings. The project already answered this for its own UI (stdlib `net/http` + a single vendored htmx), and the same discipline transfers:

- Validation endpoint: stdlib `net/http`, no framework. It is a few routes returning signed blobs; it does not need one.
- Store / checkout: lean on the payment processor's hosted checkout so their PCI-scoped page handles the card fields, and keep the vendor's own surface to a thin webhook receiver plus a status page. This pushes the heavy, fast-churning frontend onto the processor and keeps the vendor repo's own dependency list near-empty.
	- Payment integration: prefer direct HTTPS calls to the processor's API over a large SDK where practical; if an SDK is used, pick one vendored and pinned like every other dep here.
- Marketing / presence: a static site (no runtime deps) or the same stdlib+htmx pattern. No SPA framework.
- Datastore for entitlements/seats: this project's own engine is a candidate for the entitlement records themselves, keeping the dependency count low and dogfooding the product; a small embedded store (or even Postgres via stdlib `database/sql`) is the fallback. Decide when the validation scheme firms up.

## Open decisions for you

- Repo layout: A, B, or C above (recommend A).
- Whether the vendor side reuses this project's engine for its entitlement datastore, or uses something conventional.
- Payment processor choice (drives how thin the store can stay).
- Naming for the third repo.
- Whether the shared token format is a vendored module or a duplicated spec.

Once the layout is chosen, the "folder hierarchy" backlog item becomes mechanical: create the repo(s)/modules to match, with the import rules (product never imports issuance) enforced in CI.
