# 26-10-01-425 — A forwarded loopback proxy is retargeted, not preserved

- **Status**: Accepted (supersedes [0020](0020-nested-docker-trust-is-injected-by-a-runc-wrapper.md)
  §4's "never overrides" for an env value naming the sandbox's loopback
  forwarder)
- **Date**: 2026-10-01
- **Relates to**: [ADR 0020](0020-nested-docker-trust-is-injected-by-a-runc-wrapper.md)
  §5, whose loopback→bridge rewrite this extends from injected values to values
  the container already carries.

## Context

A sandbox's own processes reach the pool proxy through the loopback forwarder,
`HTTP_PROXY=http://127.0.0.1:17008`. ADR 0020 has the runc wrapper inject the
same variables into every nested container with that address rewritten to the
bridge-facing forwarder (§5), and leave alone any env name the container
already sets (§4).

Tools that forward the caller's environment defeat that. kind passes
`HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` into its node containers with `-e`,
and `docker run -e HTTP_PROXY`, compose's bare `environment:` entries and
devcontainers do the same. The container then sets the name itself, §4 keeps
the value, and the value is the sandbox's loopback — inside the container, its
own loopback, where nothing listens. In kind's case the node's containerd could
not pull a single image.

## Decision

### 1. A value naming the loopback forwarder is rewritten wherever it appears

The wrapper rewrites the loopback forwarder's address to the bridge forwarder's
in every env value the container carries, not only in the values it injects.
§4's rule protects a user's choice; this value is not one. It is the sandbox's
own proxy env copied in verbatim, and from a container with its own network
namespace it names that container's loopback, where nothing listens. Any other
value, a user-set proxy included, is still left alone.

A container sharing the sandbox's network namespace (`--network host`, which
the OCI spec shows as no `network` entry under `linux.namespaces`) is exempt:
its loopback is the sandbox's, and the value works as written.

Matching stays by address, as in 0020 §5, so `npm_config_proxy` or any other
tool-specific variable carrying the address is covered without a name list.

### 2. With no bridge forwarder published, the value is dropped

When the bridge forwarder has not yet published an address, there is nothing to
rewrite to, and the entry is removed — the same choice `proxyEnv` makes for an
injected value: a proxy set to an address nothing listens on hangs, while an
unset one fails directly and plainly.

## Alternatives considered

1. **Keep §4 and document the workaround.** Every kind user has to recreate the
   cluster with the bridge address by hand, and must first know what that
   address is — dockerd chooses it (0020 §7). Rejected: the wrapper already
   knows both addresses, and the value is unusable as it stands.
2. **Point the sandbox's own proxy env at the bridge address.** One value would
   then work everywhere, and nothing would need rewriting. Rejected: dockerd is
   socket-activated, so docker0 usually does not exist when the sandbox's
   processes receive their env; and pool-agent writes that env into
   `sandbox.json` before boot, when dockerd has not chosen the subnet.
3. **Bind the forwarder to a fixed address on `lo`, reachable from both sides.**
   Rejected for now: it reserves an address in every sandbox at every nesting
   level, the kind of fixed claim 0020 §7 moved away from, to fix what a
   rewrite at the one interception point already fixes.
4. **Rewrite only the names in the manifest's `ProxyEnvs`.** Rejected: it
   misses tool-specific copies, and is the second name list 0020 §5 exists to
   avoid.

## Consequences

- kind, compose and `docker run -e` reach the proxy from nested containers with
  no extra setup.
- A container that deliberately points at `127.0.0.1:17008` inside its own
  network namespace has that value changed. Nothing in a sandbox has reason to;
  the port is the sandbox forwarder's.
- Pods inside a kind cluster are untouched: the node's own containerd runs them
  through its own runc, so they get neither the proxy env nor the MITM CA. That
  is unchanged by this decision.
