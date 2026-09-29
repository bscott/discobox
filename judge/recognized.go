package judge

import "strings"

// Recognition names what trusted code recognized a request, or its body, as:
// a protocol, an endpoint, or the parser that read the body (ADR 26-09-26-240
// §1, §6). The version changes whenever what that code shows or says changes,
// so a stored verdict can be read against the code that described it.
type Recognition struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// The protocols a pool recognizes. A protocol is the wire protocol, whatever
// host speaks it.
const (
	// ProtocolGitReceivePack is a git push over smart HTTP: a POST to
	// …/git-receive-pack whose body is the ref updates, as pkt-lines, then a
	// packfile.
	ProtocolGitReceivePack = "git-receive-pack"
)

// The endpoints a pool recognizes. An endpoint is one operation on one API.
const (
	// EndpointGitHubFork is GitHub's POST /repos/{owner}/{repo}/forks.
	EndpointGitHubFork = "github.fork"
	// EndpointDiscoboxSandboxCreate is the discobox API's
	// POST /projects/{project}/sandboxes, reached through a pool's gate.
	EndpointDiscoboxSandboxCreate = "discobox.sandbox.create"
)

// The parsers a pool reads a body with, chosen by media type and refined by
// the protocol.
const (
	ParserText           = "text"
	ParserJSON           = "json"
	ParserForm           = "form"
	ParserMultipart      = "multipart"
	ParserGitReceivePack = ProtocolGitReceivePack
)

// guidance is what Discobox knows about a protocol or an endpoint, as the
// judge is told it: trusted sentences about what the operation means and what
// to weigh (ADR 26-09-26-240 §4). It is here, on the trusted side, and not in
// what a pool sends: a pool names what it recognized, and the words that go
// with that name are the judge package's.
//
// Guidance explains. It never authorizes: the approved purpose and host are
// the only authorization, and nothing here widens them.
var guidance = map[string][]string{
	ProtocolGitReceivePack: {
		"This is a git push over smart HTTP. The body's metadata lists the refs the push changes, read the way the server reads them: \"create\" makes a ref that does not exist yet, \"update\" moves one from its old commit to a new one, and \"delete\" removes one. When the list is all of them, the push changes those refs and nothing else. It is not all of them when \"commandsCut\" says the body was longer than Discobox read, or when \"commandsTotal\" is larger than the list; and with no metadata, \"parseError\" says why the body could not be read, and what the push changes is unknown.",
		"A body whose metadata is only \"probe\" is git asking whether the server will take a push too large to buffer. It changes nothing; the push itself follows as its own request.",
		"Force is not on the wire. `git push --force` only lets a client send an update that is not a fast-forward, and whether an update is a fast-forward depends on the repository's history, which is not in the request. A create or a delete is never a forced update. An update's metadata cannot say whether it rewrites history; weigh that only when the approved purpose rules out rewriting a ref that already exists.",
		"The packfile after the ref updates holds the objects the new commits need. It is binary, and is described by its size rather than shown.",
	},
	EndpointGitHubFork: {
		"This is GitHub's fork endpoint: it creates a copy of the repository named in the path. The body's \"organization\" is where the copy is created; with none, it is created in the account the credential belongs to. \"name\" renames the copy, and \"default_branch_only\" copies only the default branch.",
	},
	EndpointDiscoboxSandboxCreate: {
		"This creates one new discobox: a sandbox that runs an agent as the same user, on the \"prompt\" it is given, starting from the \"source\" and any \"otherSources\" named (\"none\" is an empty machine). One request is one discobox.",
		"Each of \"grants\" hands the new discobox uses of one credential — \"credential\" for a well-known one, or \"secret\" and \"envVar\" for a project secret — to \"host\" when named, for \"ttlSeconds\" when named. Its \"uses\" are the sentences the new discobox's own commands and requests will be judged against, so they are what it may do with the credential: weigh whether each is within what the approved purpose delegates, as narrow as the purpose says. \"secrets\" are credentials assigned to it outright, with no use sentence to judge against, which is broader than a grant. \"env\" names plain environment variables it sets; their values are not in the metadata and may themselves be a credential, so ask for the body when a name suggests one.",
		"\"prompt\" is only the start of what the new discobox is told, and \"promptBytes\" says how long it is; it is the discobox's task in the requester's words, not an authorization. A prompt that ends in \"…\" was cut short to fit, and says nothing about the grants. The metadata lifts what a create most often turns on, not every field (the harness config, the model, the user and the git identity are left to the body).",
		"\"grantsTotal\" is how many grants the body holds, 0 when it holds none, and absent when its grants could not be read. Every grant is in the metadata when \"grants\" lists that many and none of their \"uses\" ends in \"…\". When \"grantsTotal\" is larger or absent, or a use was cut short, ask for the body to see the rest rather than refusing for want of it.",
	},
}

// GuidanceFor is the guidance for what a request was recognized as: its
// protocol's, then its endpoint's. A name this package does not know brings
// none: a pool that recognizes something its control plane has no words for
// is judged without them rather than refused.
func GuidanceFor(request *Request) []string {
	if request == nil {
		return nil
	}
	var out []string
	for _, recognized := range []*Recognition{request.Protocol, request.Endpoint} {
		if recognized != nil {
			out = append(out, guidance[recognized.Name]...)
		}
	}
	return out
}

// valid reports whether a recognition names something in the form every
// name here takes: short, lower case, and nothing but letters, digits, dots
// and dashes. What a pool recognized is recorded and shown; it is not free
// text.
func (r *Recognition) valid() bool {
	if r == nil {
		return true
	}
	if r.Name == "" || len(r.Name) > 64 || r.Version < 1 {
		return false
	}
	return strings.Trim(r.Name, "abcdefghijklmnopqrstuvwxyz0123456789.-") == ""
}
