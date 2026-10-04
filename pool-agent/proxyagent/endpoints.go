package proxyagent

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// What a recognized endpoint lifts out of its body (ADR 26-09-26-240 §1, §3).
//
// Each describer reads a JSON body that is one whole object and returns what
// says what the operation does, beside the parser's keys, in the first ask.
// It is written against the API's wire shape rather than its generated types:
// the sandbox writes every byte, so a field of the wrong type is left out, not
// an error, and the judge can still ask for the body itself. What is lifted is
// redacted the way everything else shown is, and held to the metadata bounds
// by boundMetadata.

// liftValues lifts the named top-level scalars, the fields that say what an
// operation does when it is the rest of its path that names the target.
func liftValues(names ...string) func(map[string]json.RawMessage, parsedBody) map[string]any {
	return func(fields map[string]json.RawMessage, in parsedBody) map[string]any {
		values := map[string]any{}
		for _, name := range names {
			value, ok := scalar(fields[name], in)
			if !ok {
				continue
			}
			if credentialName(name) {
				value = redactedValue
			}
			values[name] = value
		}
		if len(values) == 0 {
			return nil
		}
		return map[string]any{"values": values}
	}
}

// describeSandboxCreate is what a discobox API create says about the discobox
// it makes: the credentials it hands the new discobox — as grants of approved
// uses, as secrets assigned outright, and the names of the plain variables it
// sets, which may carry one too — what it is told to do, what it
// runs, and where it starts from. The grants are what a use of the discobox
// credential is usually approved around ("one discobox per issue, each
// granted its own branch"), and they sit last in the body, after a prompt
// that can be kilobytes long, so they are lifted rather than left to a round
// that shows the body. Their count is said even when it is none, so a create
// that grants nothing reads as that, not as grants the metadata left out; a
// grants field that cannot be read is not counted at all.
func describeSandboxCreate(fields map[string]json.RawMessage, in parsedBody) map[string]any {
	out := map[string]any{}
	if grants, ok := sandboxGrants(fields["grants"], in); ok {
		out["grantsTotal"] = len(grants)
		if len(grants) > 0 {
			out["grants"] = grants
		}
	}
	for name, as := range map[string]string{"harnessName": "harness", "poolId": "pool"} {
		if value, ok := scalar(fields[name], in); ok {
			out[as] = value
		}
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(fields["config"], &config) != nil || config == nil {
		return out
	}
	for _, name := range []string{"harnessMode", "image", "name", "description"} {
		if value, ok := scalar(config[name], in); ok {
			out[name] = value
		}
	}
	var prompt []string
	if json.Unmarshal(config["prompt"], &prompt) == nil && len(prompt) > 0 {
		// The start of what the new discobox is told to do, and how much of it
		// there is: enough to say which work it is for, which the grants are
		// scoped to. The rest is the body's to show.
		text := strings.Join(prompt, " ")
		out["prompt"] = in.redact(text)
		out["promptBytes"] = len(text)
	}
	if secrets := sandboxSecrets(config["secrets"], in); secrets != nil {
		out["secrets"] = secrets
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(config["env"], &env) == nil && len(env) > 0 {
		// Plain variables, which can carry a credential as well as a grant
		// can: named here so the judge knows they are set, their values the
		// body's to show.
		var names []any
		for _, name := range slices.Sorted(maps.Keys(env)) {
			names = append(names, in.redact(name))
		}
		out["env"] = names
	}
	if source := sandboxSource(config["source"], in); source != nil {
		out["source"] = source
	} else if len(config["source"]) == 0 || string(config["source"]) == "null" {
		out["source"] = "none"
	}
	var references map[string]json.RawMessage
	if json.Unmarshal(config["sourceCodeReferences"], &references) == nil && len(references) > 0 {
		var others []any
		for _, name := range slices.Sorted(maps.Keys(references)) {
			if source := sandboxSource(references[name], in); source != nil {
				others = append(others, source)
			}
		}
		if others != nil {
			out["otherSources"] = others
		}
	}
	return out
}

// describeSourcePushed is which commit a discobox's source-push report says
// was pushed for each of its sources: what the discobox will start from.
func describeSourcePushed(fields map[string]json.RawMessage, in parsedBody) map[string]any {
	var sources map[string]json.RawMessage
	if json.Unmarshal(fields["sources"], &sources) != nil || len(sources) == 0 {
		return nil
	}
	pushed := map[string]any{}
	for slug, raw := range sources {
		if commit, ok := scalar(raw, in); ok {
			pushed[in.redact(slug)] = commit
		}
	}
	return map[string]any{"sources": pushed}
}

// sandboxGrants is each grant a create hands the new discobox: the credential,
// where it may go, and the sentences each use was granted as, which are what
// the new discobox will be judged against. It reports whether it could read
// them: no grants field, or a null one, is none.
func sandboxGrants(raw json.RawMessage, in parsedBody) ([]any, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	var grants []map[string]json.RawMessage
	if json.Unmarshal(raw, &grants) != nil {
		return nil, false
	}
	out := make([]any, 0, len(grants))
	for _, grant := range grants {
		described := map[string]any{}
		for name, as := range map[string]string{
			"wellKnownId": "credential", "secretId": "secret", "envVar": "envVar",
			"host": "host", "grantTTLSeconds": "ttlSeconds",
		} {
			if value, ok := scalar(grant[name], in); ok {
				described[as] = value
			}
		}
		var uses []map[string]json.RawMessage
		if json.Unmarshal(grant["uses"], &uses) == nil {
			var said []any
			for _, use := range uses {
				if description, ok := scalar(use["description"], in); ok {
					said = append(said, description)
				}
			}
			if said != nil {
				described["uses"] = said
			}
		}
		out = append(out, described)
	}
	return out, true
}

// sandboxSecrets is each secret a create assigns the new discobox outright,
// by its variable and which secret it is. An inline value is said to be one
// and never shown.
func sandboxSecrets(raw json.RawMessage, in parsedBody) []any {
	var secrets []map[string]json.RawMessage
	if json.Unmarshal(raw, &secrets) != nil || len(secrets) == 0 {
		return nil
	}
	out := make([]any, 0, len(secrets))
	for _, secret := range secrets {
		described := map[string]any{}
		for _, name := range []string{"env", "secretId", "host"} {
			if value, ok := scalar(secret[name], in); ok {
				described[name] = value
			}
		}
		if _, inline := secret["value"]; inline {
			described["value"] = "inline, not shown"
		}
		out = append(out, described)
	}
	return out
}

// sandboxSource is where a source starts from: the repository, as a local
// directory or a URL, and the commit and ref it is checked out at.
func sandboxSource(raw json.RawMessage, in parsedBody) map[string]any {
	var source map[string]json.RawMessage
	if json.Unmarshal(raw, &source) != nil || source == nil {
		return nil
	}
	out := map[string]any{}
	for _, name := range []string{"slug", "kind", "url", "localDirectory"} {
		if value, ok := scalar(source[name], in); ok {
			out[name] = value
		}
	}
	var checkout map[string]json.RawMessage
	if json.Unmarshal(source["checkout"], &checkout) == nil {
		for _, name := range []string{"refName", "commit"} {
			if value, ok := scalar(checkout[name], in); ok {
				out[name] = value
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scalar is one JSON scalar, redacted when it is text. Anything else — an
// object, an array, a field that is absent — is not one.
func scalar(raw json.RawMessage, in parsedBody) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	switch typed := value.(type) {
	case string:
		return in.redact(typed), true
	case bool, float64:
		return typed, true
	default:
		return nil, false
	}
}
