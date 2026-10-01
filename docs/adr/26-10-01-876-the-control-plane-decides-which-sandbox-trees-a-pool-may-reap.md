# 26-10-01-876 — The control plane decides which sandbox trees a pool may reap

- **Status**: Accepted (supersedes [0022](0022-sandbox-deletion-is-archive-then-confirmed-purge.md)
  §4's closing paragraph, "the reaper stays, unchanged in purpose", and §6's
  "what the reaper skips"; and [0123](0123-a-discobox-is-exported-as-its-spec-and-its-durable-tree.md)
  §3's "exactly as it does for a sandbox whose container was lost out of band" —
  a died import's tree is still collected, because it has no row, and a
  container-lost one no longer is)
- **Date**: 2026-10-01
- **Relates to**: [ADR 0022](0022-sandbox-deletion-is-archive-then-confirmed-purge.md)
  §3, which keeps a sandbox's row until the pool agent confirms its tree is
  gone; [ADR 0017](0017-resource-state-is-desired-and-observed-with-no-operations.md)
  §4, under which a settled failure waits for a user's repair;
  [ADR 0123](0123-a-discobox-is-exported-as-its-spec-and-its-durable-tree.md)
  §3, whose import restores a tree before its row exists.

## Context

The pool agent reaps sandbox durable trees
(`projects/{project}/pools/{pool}/sandboxes/{sandbox}`) with
`reapDeadSandboxVolumes`. It decides a tree is dead from its own view alone:
no container carries that sandbox ID, so it writes a `.discobox-orphaned-at`
tombstone and removes the tree 24 hours later. The archive marker is the only
exemption. ADR 0022 kept this, "unchanged in purpose", as accident recovery for
a container removed out of band or lost while the pool was down.

That premise is wrong: a missing container is not evidence that a sandbox is
gone. A sandbox the control plane still holds lost its whole tree — home,
sources, the harness's session — this way:

1. `CreateSandbox` on a spec drift removed the old container before building
   the new one, and the build failed. The sandbox was left with a tree and no
   container.
2. The reconciler recorded a settled failure. Under ADR 0017 §4 a settled
   failure is converged and waits for a user's repair, so nothing recreated
   the container.
3. A day later the reaper deleted the tree. The control plane still listed the
   sandbox as `present`; when the user repaired it, it was rebuilt from its
   original spec into an empty tree.

The failure is not specific to step 1. Any path that leaves a held sandbox
without a container — a failed rebuild, a daemon that lost its containers, a
`docker rm` by hand — becomes data loss once the sandbox stays that way for a
day, and a settled failure is precisely a sandbox that stays that way.

The pool agent is not the authority on which sandboxes exist; the control plane
is. ADR 0022 §3 already made deletion an explicit, confirmed `DeleteSandbox`
that removes the tree, so a sandbox the control plane deleted has no tree left
for a reaper to find. What the reaper can still usefully collect is a tree with
no row at all: an import that died between restoring the tree and creating the
row (ADR 0123 §3), a row removed without its delete reaching this pool.

## Decision

### 1. A tree is reaped only when the control plane does not hold its sandbox

The control plane answers, per pool, the set of sandbox IDs it holds: every
sandbox row on that pool, in any state — present, failed, archived, and mid-
delete until `DeleteSandbox` confirms (ADR 0022 §3). The pool agent reaps a tree
only when its ID is outside that set, and only once it has been outside it for
the retention window (24 hours). A tree back in the set has its clock cleared.

Whether a container exists plays no part. A held sandbox's tree is kept
indefinitely, however long its container has been gone.

The retention window stays, and its purpose changes: it is recovery from a bad
or partial answer — a restored database, a bug in the query — rather than from
a lost container. It also covers the moment an import has restored its tree and
not yet created its row.

### 2. The pool agent asks; nothing answers means nothing is reaped

The set is read by the pool agent from the control plane
(`GET /api/pools/{poolId}/sandboxes`, the pool's own signed assertion), at the
moment it is about to reap, rather than pushed to it. It lists the trees first
and asks second, so a tree it is judging existed before the answer was taken;
a sandbox's row is written before any create reaches the pool.

If the agent cannot get an answer, it reaps nothing on that pass. No answer is
never read as an empty set.

The set is exactly as wide as the tree it authorizes: one pool's sandboxes,
scanned against that pool's `sandboxes` directory, the same rule `pool-sync`
follows for whole pools.

#### Rejected: carry the set on `pool-sync`

`pool-sync` is the control plane pushing the authoritative pool set, and adding
the pool's sandboxes to it would have been one field. It is sent when the pool
reconciles, which once a pool is ready is rarely. A sandbox created after the
last sync would be outside the set the agent holds, and its tree would be reaped
a day later — unless the agent also counted the sandboxes it had been asked to
create since, which hands authority back to the agent's local view. Pushing on
every sandbox change instead is a second delivery channel kept in step with the
first. Reading at the moment of the decision has neither problem.

#### Rejected: keep container-absence reaping, with a marker for failed sandboxes

The archive marker already exempts one kind of held, container-less sandbox; a
second marker written when a build fails would have exempted the one that lost
data. It leaves the agent the authority on what exists, and so leaves every
other way of losing a container as a way of losing data: the marker covers the
paths someone remembered to mark, and the incident was a path nobody had.

### 3. The archive marker stays, for what it is still for

An archived sandbox is held, so the set protects it and the reaper no longer
reads the marker. The marker remains what refuses an on-demand start and what
makes `ArchiveSandbox` idempotent (ADR 0022 §§5–6).

### 4. A spec change does not drop the container before it can build the next

`CreateSandbox` resolves the image a drifted sandbox needs — the step a re-pin
fails at — before it stops and removes the existing container. A sandbox whose
new image cannot be obtained keeps the container it had. This narrows how often
a held sandbox is left without a container; §1 is what makes that state safe.

## Consequences

- A sandbox the control plane holds keeps its data for as long as it is held.
  A settled failure can wait for its repair indefinitely, and a repair rebuilds
  against the tree it had.
- A pool agent that cannot reach the control plane reclaims no sandbox trees.
  Disk held by unreachable agents' garbage waits for the connection, which is
  the safe direction.
- A control-plane database restored from an older backup still knows the pool,
  but not the sandboxes created after the backup. Their trees are outside the
  set from the first pass and are reaped a day later. The retention window is
  the margin against that, and it is only a margin if someone sees it open, so
  the agent logs a warning naming each tree the first time it is found outside
  the set. (A database lost outright no longer knows the pool or its key; the
  agent gets no answer and reaps nothing.)
- Trees tombstoned under the old rule carry `.discobox-orphaned-at`; the new
  clock is a different file, so no tree is reaped on a clock started by
  container absence.
