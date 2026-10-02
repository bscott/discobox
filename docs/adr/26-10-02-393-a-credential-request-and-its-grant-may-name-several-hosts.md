# 26-10-02-393 — A credential request and its grant may name several hosts

- **Status**: Proposed (supersedes [0031](0031-agent-credentials-are-a-portable-protocol-with-ephemeral-sentinels.md)
  §1's one `host` per request and §5's one concrete host per grant)
- **Date**: 2026-10-02
- **Relates to**: [ADR 0031](0031-agent-credentials-are-a-portable-protocol-with-ephemeral-sentinels.md)
  §§1, 5, which gave a request and its grant one host;
  [ADR 26-10-01-240](26-10-01-240-a-swapped-request-records-the-secrets-it-spent.md)
  §3, which holds that a pool agent, a CLI, and the server they talk to are
  upgraded separately.

## Context

An agent credentials request names one host, and the grant approving it is
limited to that host and the hosts beneath it (`hostscope.Covers`). Every
check downstream reads that one host: the grant lookup a resolve makes, the
activation the pool agent mints for a use, the use the judge is asked about,
the delegation a discobox approves under.

Some tools send one credential to two sites that share no parent. Copilot CLI
authenticates with one GitHub token and sends it to `api.github.com` and to
`api.githubcopilot.com`. Today an agent has three options and all are wrong:

- Two requests and two grants — but the CLI runs a command under one use,
  and the value the pool mints for it is pinned to that use's grant's host, so
  the process holding it is refused at the other site.
- A wildcard grant, which this flow refuses to mint (ADR 0031 §5) for good
  reason.
- A grant for the common parent, which for these two is `com` and is refused
  as too broad.

## Decision

### 1. A request and its grant carry a list of hosts

`SecretRequest.Hosts` and `SecretGrant.Hosts` replace `Host`. A grant covers a
destination when any of its hosts covers it, and an empty list remains the
wildcard — still minted only by an explicit `secret grant create`, never by
approving an agent's ask. Hosts are normalized the one way `hostscope` does,
deduplicated, and kept in the order they were asked for.

Where one grant has to be picked among several that cover a destination, its
specificity is that of its closest host.

### 2. Every check asks "does any host cover this destination?"

The grant lookup, `ApprovedUse`, the pool agent's activation check, and the
delegation check all read the list. An approved use is reported to the judge
with the grant host that covers the destination it is going to, so the judge
is told where *this* request is approved for, not a list it must match itself.

A grant is checked against its secret host by host: each must sit inside the
secret's binding. A secret binding stays one host. A credential that genuinely
spans unrelated sites is an unbound secret, as the grant-host guard already says,
and the approval can release the binding in the same act (`secretHost`).

A delegated approval's hosts must each be covered by the delegation grant's.

### 3. Two requests ask the same when they name the same set of hosts

The pending-request dedup compares the host lists as sets: approving either
would grant the same thing, so a retry that lists them in another order is a
retry.

### 4. `host` stays on every wire beside `hosts`

ADR 26-10-01-240 §3 holds that the pool agent, the CLI, and the in-sandbox CLI
are upgraded separately from the server, and the agent credentials protocol is
portable and versioned (`v1`). So `hosts` is added, not substituted:

- **In what a reader receives** — the protocol's `list`, the pool's
  `sandbox-credentials`, the server's `SecretGrant` and `SecretRequest` —
  `hosts` is every host and `host` is the first. An older pool agent reads one
  host and fails closed: it pins an activation to the first host and refuses
  the swap at the second. An older CLI or console refuses a `SecretGrant` or
  `SecretRequest` carrying a field it does not know, so `secret grant list`,
  `secret request list`, and the approval inbox fail against a newer server
  until the client is upgraded — the cost every added response field already
  carries, and one that fails visibly rather than wide.
- **In what a writer sends** — the protocol's `request`, the pool's
  `sandbox-credential-requests`, the server's approve and grant-create bodies —
  `host` and `hosts` may both be given, and the hosts are `host` followed by
  `hosts`. An older writer that sends `host` alone asks for one host, as it
  meant to.

This is the only place both spellings exist. The model, the store, and the code
between them carry the list alone.

### 5. A well-known credential may list more hosts than it asks for

`com.github.api` gains `githubcopilot.com`. A well-known credential's `Hosts`
already meant "where it may be sent", and an ask by ID that names no host still
asks for the first alone, so nothing that asks for GitHub today is widened to
Copilot. An ask by ID that names hosts has each checked against the list, so
`com.github.api` asked for at `api.github.com` and `api.githubcopilot.com` is
one request, one grant, and one use.

### 6. The column is migrated, not recreated

`secret_requests` and `secret_grants` gain a `hosts` column (a JSON list).
Migration backfills it from `host` — `[host]` where `host` is set, empty where
it is not — and then drops `host` with `dropRetiredColumn`, both guarded by
whether `host` still exists, so the step is idempotent.

Downgrading the server past this is not supported. An older server's
`AutoMigrate` re-adds `host` empty, and empty is the wildcard, so every grant it
reads would be unscoped. Keeping `host` written with the first host would make a
downgrade fail closed instead, at the price of a column the server writes and
never reads; it was weighed and declined.

## Alternatives rejected

- **Two grants and two uses, one per host.** The CLI runs a command under one
  use and puts one value in one variable, and that value is pinned to one
  grant's host, so a single process cannot carry a value valid at both. Making
  one value stand for two uses at once is a larger change to the protocol than
  a list, and the person approving is still asked twice about one credential.
- **Approve at the common parent.** For the case that motivates this, there is
  none short of a public suffix, which `hostscope.TooBroad` refuses.
- **A list on the secret binding too.** It would let a GitHub secret say "this
  is for github.com and githubcopilot.com". Deferred: an unbound secret already
  expresses a credential that spans sites, the approval can release a binding
  in the same write, and the binding is part of the secret's unique index,
  which a list would have to leave. Revisit if releasing bindings to use
  multi-host grants becomes the common path rather than the exception.
- **Replace `host` with `hosts` on the wire.** Smaller, and it would break
  every older pool agent and in-sandbox CLI the moment the server is upgraded:
  an older writer's `host` would be dropped, and an older reader would see no
  host and either refuse every use or — worse, for a reader that treats an
  empty host as the wildcard — accept any.

## Consequences

- An agent asks once for a credential it sends to several sites, and a person
  approves one grant for all of them.
- A GitHub secret bound to `github.com` refuses a grant for
  `githubcopilot.com`; the approval names the remedy, releasing the binding.
- The approval inbox, `secret grant list`, and the console show a grant's
  hosts joined, and `--host` on the CLI repeats.
- Until a pool agent is upgraded, a multi-host grant works at its first host
  only.
