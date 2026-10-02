package agentcreds_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/discobox-ai/discobox/agentcreds"
)

// The hosts are host, then hosts, without repeats, so a client that names one
// host either way, and one that predates the list, ask for what they meant
// (ADR 26-10-02-393 §4).
func TestAllHostsIsHostThenHosts(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{"host alone, as a v1 client sends it", `{"host":"api.github.com"}`, []string{"api.github.com"}},
		{"hosts alone", `{"hosts":["api.github.com","githubcopilot.com"]}`, []string{"api.github.com", "githubcopilot.com"}},
		{"both, repeating the first", `{"host":"api.github.com","hosts":["api.github.com"," githubcopilot.com",""]}`, []string{"api.github.com", "githubcopilot.com"}},
		{"neither", `{}`, nil},
	} {
		var body agentcreds.RequestBody
		if err := json.Unmarshal([]byte(tc.body), &body); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := body.AllHosts(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: AllHosts = %q, want %q", tc.name, got, tc.want)
		}
	}
	credential := agentcreds.Credential{Host: "api.github.com", Hosts: []string{"api.github.com", "githubcopilot.com"}}
	if got := credential.AllHosts(); !slices.Equal(got, []string{"api.github.com", "githubcopilot.com"}) {
		t.Errorf("Credential.AllHosts = %q", got)
	}
}
