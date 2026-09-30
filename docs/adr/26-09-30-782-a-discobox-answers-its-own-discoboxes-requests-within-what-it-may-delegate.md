# 26-09-30-782 — A discobox answers its own discoboxes' requests, within what it may delegate

- **Status**: Proposed
- **Date**: 2026-09-30
- **Supersedes in part**: [0140](0140-a-discobox-reaches-the-discobox-api-through-its-pool-with-a-fixed-role.md)
  §4's pre-assigned grants on a create from a sandbox, its secret requests
  answered "with any project secret", and its secrets list in the role; §5's
  "the server reads no grant"; its rejections of "Authorize by who created a
  discobox" (for answering requests) and "The server evaluates grants and their
  uses"; and settles its Deferred "Holding a sandbox to what it was given".
  [26-09-24-630](26-09-24-630-a-discobox-delivers-the-source-of-the-discoboxes-it-creates.md)
  §4's `--grant` on `discobox new`, from a sandbox.
- **Relates to**: [26-09-30-854](26-09-30-854-a-request-is-judged-against-the-command-its-sentinel-was-minted-for.md),
  which gives the request judge the command; this ADR takes delegation away
  from the request judge altogether.

## Context

ADR 0140 lets a lead discobox holding `ai.discobox.sandbox` create workers,
give them credentials, and answer their requests, and leaves every limit on
that to the request judge: "Whether a call fits the use it was made under is
the judge's question" (§5), with bounding a lead to its own delegation grants
deferred "with the judge" (Deferred). The judge exists now, and it cannot hold
that bound:

- **It does not see the facts.** Whether a request came from one of the lead's
  workers, which delegation grants the lead holds, and what they allow are
  rows in the control plane. The judge sees an HTTP request — `GET
  /secret-requests/<id>`, a create body with `"grants"` — and a sentence.
- **It reads delegation as prose, twice nested.** A create's grants are use
  sentences inside a request judged against a use sentence. On 2026-09-29 the
  judge refused a create that carried no grants because it took a clipped
  prompt for hidden ones, then refused another because the prompt told the new
  discobox not to push.
- **It varies.** The same create was allowed on one run and refused on the next.

So in practice a lead either cannot delegate, or can hand any project secret
to any discobox, bounded only by the person who approved its
`ai.discobox.sandbox` (0140 Consequences). Delegation is authority over
credentials; it has to be decided where the grants are.

## Decision

### 1. A create from a discobox carries no grants

A discobox created by a discobox starts with no credential uses. It asks for
what its work needs, through `discobox-access`, when it knows. A create from a
sandbox that carries `grants` is refused, as one carrying inline secrets
already is. A person may still pre-assign grants.

### 2. A discobox answers only the requests of discoboxes it created

The server scopes the sandbox role's secret-request routes — list, get,
approve, deny — to requests whose discobox the caller created
(`createdBySandboxId`), as it already scopes source delivery (26-09-24-630 §2).
Other requests are not visible to it. A person answers every request, as
today, and a request for `ai.discobox.sandbox` is still only a person's to
approve.

### 3. Approving is bounded by the approver's delegation grants

The server mints the grant only within a live delegation grant
(`purpose: delegate`) the approving discobox holds:

- **the same credential** — the delegation grant's secret, or the well-known
  credential it names; the approver does not choose among project secrets, so
  the sandbox role loses its secrets list;
- **a host within** the delegation grant's host;
- **an expiry no later** than the delegation grant's; with no lifetime given,
  the lifetime the request asked for, capped there;
- **uses within** the delegation grant's uses. Whether one sentence is within
  another is a reading, not a comparison, so the server asks the project's
  judge (838 §1–2) as a delegation job: the delegation grant's uses and the
  uses being approved, both authorization, no request evidence. Anything but
  an explicit yes refuses the approval.

A grant made this way stays independent once made (0140's rejection of
cascading lifetimes stands); its expiry already ends no later than the
delegation it came from.

### 4. Approving is one request

With the secret taken from the delegation grant and the lifetime defaulted on
the server, `discobox secret request approve <id>` sends one `POST` and reads
nothing first. The request judge (under 26-09-30-854) sees the approval the
command names; the server decides whether it is within what the approver may
delegate.

## Why 0140's rejections no longer hold

**"Authorize by who created a discobox."** 0140 rejected it as a tree built by
position rather than by whether a call fits the work, and deferred the bound to
the judge. The judge cannot see who created a discobox; the server records it
and already authorizes source delivery by it. This ADR uses it for one thing —
whose requests a discobox may answer — and cascades nothing: no archive, no
purge, no inherited grants.

**"The server evaluates grants and their uses."** Rejected for splitting the
judgement of what a use allows between the server and the judge. The split
here is along what each can know: the server checks identity, credential,
host, and expiry, which are facts; the judge still reads sentences, but two
authorization sentences on trusted ground rather than a sentence against an
HTTP request.

## Alternatives rejected

**Keep delegation in the request judge** (0140 as written). It lacks the facts
and reads delegation as nested prose; see Context.

**Let a discobox pre-assign grants on create, bounded as §3 bounds approval.**
Two paths to the same authority, one of them inside a create body the request
judge must also read. The worker's own request says what it needs once it
knows, and one path is checked once.

**Show the request judge the approver's delegation grants.** It moves the
server's facts into a model's reading of an HTTP request, and still leaves the
decision to vary.

**Check only credential, host, and expiry.** A delegation grant's uses would
be decoration: a lead delegated "read issues in org/repo" could approve
"push to main".

**A tree of discoboxes with inherited limits.** 0140's rejection stands. One
level — the creator answers its own discoboxes' requests — is all delegation
needs.

## Consequences

- The sandbox role changes: create refuses `grants`; the secrets list leaves
  it; secret-request routes are scoped to discoboxes the caller created.
- A lead needs a delegation grant for each credential it will hand on, which a
  person approves once. Without one, its workers' requests wait for a person.
- `discobox new --grant` and `--json` `"grants"` are refused from a discobox;
  the in-box skills stop teaching them and teach approving instead.
- The approve API defaults an omitted lifetime to the one the request asked
  for, for every caller, within the secret's limit — which is what the window
  already opens on, so the CLI stops reading the request first.
- The judge gains a delegation job kind, and `PromptVersion` changes with it.
- Grants already pre-assigned by discoboxes stay as they are.
