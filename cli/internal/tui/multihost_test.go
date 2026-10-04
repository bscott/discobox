package tui

import (
	"strings"
	"testing"
)

// A request naming several hosts is shown with all of them, and a credential
// stored on the spot to answer it is bound to the site they share — or to
// nothing, when they share none, since a binding is one host and every host of
// the grant must sit inside it (ADR 26-10-02-393 §2).
func TestARequestForSeveralHostsIsBoundToWhatTheyShare(t *testing.T) {
	for _, tc := range []struct {
		hosts   []string
		where   string
		binding string
	}{
		{[]string{"api.github.com"}, "api.github.com", "api.github.com"},
		{[]string{"api.github.com", "uploads.github.com"}, "api.github.com, uploads.github.com", "github.com"},
		{[]string{"api.github.com", "api.githubcopilot.com"}, "api.github.com, api.githubcopilot.com", ""},
	} {
		req := CredentialRequest{Host: tc.hosts[0], Hosts: tc.hosts}
		if got := req.where(); got != tc.where {
			t.Errorf("where(%q) = %q, want %q", tc.hosts, got, tc.where)
		}
		if got := req.binding(); got != tc.binding {
			t.Errorf("binding(%q) = %q, want %q", tc.hosts, got, tc.binding)
		}
	}
	// A trust names its endpoint as Host alone.
	if got := (CredentialRequest{Host: "kube.internal:6443"}).where(); got != "kube.internal:6443" {
		t.Errorf("a trust's where = %q", got)
	}
}

// A secret bound to github.com answers a request for GitHub alone, and is asked
// about before it is granted for Copilot's site too.
func TestASecretAnswersOnlyWhenItsBindingCoversEveryHost(t *testing.T) {
	secret := Secret{Name: "github", Type: "token", Host: "github.com"}
	if got := secretDetail(secret, []string{"api.github.com"}); strings.Contains(got, "asks before") {
		t.Errorf("detail = %q, want it to answer api.github.com plainly", got)
	}
	if got := secretDetail(secret, []string{"api.github.com", "api.githubcopilot.com"}); !strings.Contains(got, "asks before") {
		t.Errorf("detail = %q, want it to ask before answering a host outside its binding", got)
	}
}

// For a request a bound secret cannot answer in full, the unbound secret that
// answers it plainly is offered before the one whose binding would have to be
// released.
func TestThePickerRanksBySecretsThatCoverEveryHost(t *testing.T) {
	req := CredentialRequest{Host: "api.github.com", Hosts: []string{"api.github.com", "api.githubcopilot.com"}}
	bound := Secret{ID: "sec-bound", Name: "github", Host: "github.com"}
	unbound := Secret{ID: "sec-free", Name: "github-anywhere"}
	if got := secretsForRequest([]Secret{bound, unbound}, req); got[0].ID != unbound.ID {
		t.Fatalf("picker order = %s, %s; want the unbound secret first", got[0].ID, got[1].ID)
	}
	one := CredentialRequest{Host: "api.github.com", Hosts: []string{"api.github.com"}}
	if got := secretsForRequest([]Secret{unbound, bound}, one); got[0].ID != bound.ID {
		t.Fatalf("picker order = %s, %s; want the secret bound to the site first", got[0].ID, got[1].ID)
	}
}
