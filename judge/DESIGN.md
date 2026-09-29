# judge

The question Discobox puts to a model before a credential is used, and the
answer it will accept back. See
[ADR 26-09-22-838](../docs/adr/26-09-22-838-a-dedicated-pool-harness-judges-commands-and-credential-bearing-requests.md)
for the whole design; this package is its contract, and holds nothing that runs
it.

## What is here

| File | What it holds |
| --- | --- |
| `judge.go` | `Job` — a command or an observed request, judged against one approved use — its bounds, and the JSON prompt it becomes. |
| `system.go` | `System`, the words the judge is given, and `PromptVersion`, which changes with them. |
| `verdict.go` | `Answer`, `Need`, `Schema`, and `Decode`: what Discobox will accept as a verdict. |
| `standing.go` | `Standing`, `Route`, and `Job.Admits`: an allow the judge asks to let stand for a route, and whether it may. |
| `recognized.go` | `Recognition`, the names of the protocols, endpoints and parsers a pool recognizes, and `GuidanceFor`: the trusted words that go with each name. |

## The rules it exists to keep

**The trusted side owns the question.** A caller supplies evidence and the
approved use. The prompt, the schema, the role, and the bounds are here, so
that what is asked is the same wherever the asking happens, and a caller cannot
choose a kinder question.

**Evidence is data.** `Job` is marshalled to JSON, so a request body that
spells out a verdict and then gives fresh instructions arrives as one string in
one field. `Purpose` and `Host` are the authorization; nothing else can widen
them.

**Only an explicit allow is one.** `Decode` refuses anything that could be read
as permission nobody gave: prose around the object, a second object, a key said
twice at any depth (`encoding/json` would take the last), a field nobody
defined, an answer that both decides and asks, and an answer with no reason.
A failure to decode is not an allow, and callers treat it as a refusal.

**A body is described in one shape; its bytes are asked for, not sent**
(ADR 26-09-26-240). A request job describes its body — media type, length,
and, when a parser recognized it, the `Parser`, its `Metadata` (one JSON
object, at most `MaxMetadataBytes`) or its `ParseError` — and carries its
`Content` only once the judge answers with `Need`, which names no form: the
parser decides how a body is written, not the judge. That is why `Answer` has
three outcomes rather than two. `System` tells the judge never to refuse for want of a body it
may still ask to see: an operation that lives in the body is otherwise refused
on the first round's description alone. `Budget` caps what may be shown at
`MaxBodyBytes` whatever the judge names, `Content` is present — even empty —
exactly when it was shown, `Body.Missing` says what is not being shown and why,
and `Body.Answers` reports an ask that would change nothing, which is a judge
that has decided nothing.

**Guidance is the trusted side's.** A request names what a pool recognized it
as (`Request.Protocol`, `Request.Endpoint`); the words that go with a name are
here (`GuidanceFor`), and the control plane puts them in `Job.Guidance`. A pool
sends names, never sentences, so nothing a pool or a request says becomes
Discobox speaking. A name nobody wrote guidance for brings none, so a pool
newer than its control plane is judged without guidance, not refused.
Guidance explains; the system prompt says it never authorizes.

**A wrapper prints the verdict and nothing else.** `Decode` takes one JSON
object and no prose around it, which is a requirement on every harness image's
`discobox-prompt`: a wrapper that frames its answer in a transcript cannot
judge. See [`harness/DESIGN.md`](../harness/DESIGN.md) for the wrapper
contract.

**The judge proposes a standing allow; Discobox admits it.** An allow may carry
a `Standing` route: net/http pattern syntax, one method and an exact path
(ADR 26-09-25-428). `Decode` refuses a route that does not parse, or one beside
anything but an allow. `Job.Admits` keeps only a first-round route decided
before the body's content was shown, on a request whose operation is not in its
body (`Request.OperationInBody`: a recognized protocol, an endpoint that reads
its body — every endpoint but the reads `operationOutsideBody` names, so one
this package does not know is read as reading its body — or a body its parser
could not read) — a JSON object's keys are its
shape, not its operation, and do not stop one; the control plane asks the same
of every request a standing allow would answer, since a route says nothing
about a body — standing
for some time, with a literal segment, that covers its own request; `Duration`
caps it at `MaxStanding`. A route matches the method and the unescaped path
segments and nothing else. A path with a dot or empty segment, or a segment
that unescapes to a slash or backslash, matches no route, because the upstream
may resolve it somewhere the route never named. The host, the discobox, and the use are never the
judge's to name.

**Rounds are bounded.** `MaxRounds` asks in total, inside one `Timeout` for the
whole exchange, because a request is held open while the judge thinks.
`ReachWait` comes before it: how long the control plane waits for the judge's
discobox to become reachable, which every hop bounding the exchange allows for. A
command job is asked once: there is nothing further to show.
