package proxyagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/discobox-ai/discobox/agentcreds"
	"github.com/discobox-ai/discobox/layout"
	"github.com/discobox-ai/discobox/proxy"
)

// fakeControlPlane answers list and sandbox-credential-verdicts the way the
// real control plane does, and records what it was sent — which is the half
// of ADR 0091's guarantee this package owns: that a verdict is sent, and sent
// before a value is ever minted.
type fakeControlPlane struct {
	mu           sync.Mutex
	credentials  []credentialDoc
	verdictCalls []recordCredentialVerdictDoc
	verdictErr   error
	requests     []createCredentialRequestDoc
}

func newFakeControlPlane(t *testing.T, credentials []credentialDoc) (*controlPlaneCredentials, *fakeControlPlane) {
	t.Helper()
	withTestRoot(t)
	fake := &fakeControlPlane{credentials: credentials}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sandbox-credentials"):
			_ = json.NewEncoder(w).Encode(listCredentialsDoc{Credentials: fake.credentials})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sandbox-credential-requests"):
			fake.mu.Lock()
			defer fake.mu.Unlock()
			var body createCredentialRequestDoc
			_ = json.NewDecoder(r.Body).Decode(&body)
			fake.requests = append(fake.requests, body)
			_ = json.NewEncoder(w).Encode(credentialRequestStatusDoc{RequestID: "req-1", Status: agentcreds.StatusPending})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sandbox-credential-verdicts"):
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if fake.verdictErr != nil {
				http.Error(w, fake.verdictErr.Error(), http.StatusInternalServerError)
				return
			}
			var body recordCredentialVerdictDoc
			_ = json.NewDecoder(r.Body).Decode(&body)
			fake.verdictCalls = append(fake.verdictCalls, body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	if err := WriteResolveContext(testProjectID, testPoolID, server.URL, "tok"); err != nil {
		t.Fatalf("write resolve context: %v", err)
	}
	broker := &controlPlaneCredentials{
		contextPath: layout.ProxyResolveContextFile(testProjectID, testPoolID),
		client:      server.Client(),
	}
	return broker, fake
}

func allowVerdict() agentcreds.Verdict {
	return agentcreds.Verdict{Allow: true, Reason: "matches the approved use", Role: "judge", Prompt: "..."}
}

// The verdict must reach the control plane before the value does: ADR 0091's
// whole guarantee is that a credential is never issued without a record of
// why, and that only holds if the record actually lands.
func TestGetRecordsTheVerdictBeforeMintingTheValue(t *testing.T) {
	broker, fake := newFakeControlPlane(t, []credentialDoc{{
		EnvVar: "GITHUB_TOKEN", Hosts: []string{"api.github.com"}, Sentinel: "STABLE-1",
		Uses: []credentialUseDoc{{UseID: "use-1", Description: "open a PR"}},
	}})
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: newActivations()}

	out, err := b.Get(context.Background(), agentcreds.UseBody{
		UseID:   "use-1",
		Command: []string{"gh", "pr", "create"},
		Verdict: allowVerdict(),
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.EnvVar != "GITHUB_TOKEN" || out.Value == "" {
		t.Fatalf("response = %#v, want a minted value", out)
	}
	if len(fake.verdictCalls) != 1 {
		t.Fatalf("verdict calls = %d, want exactly one", len(fake.verdictCalls))
	}
	call := fake.verdictCalls[0]
	if call.SandboxID != "sb-1" || call.UseID != "use-1" || call.Volunteered {
		t.Fatalf("recorded verdict = %#v, want it scoped to this sandbox/use and not volunteered", call)
	}
	if !call.Verdict.Allow || call.Verdict.Role != "judge" {
		t.Fatalf("recorded verdict = %#v, want the allow verdict carried through", call.Verdict)
	}
}

func TestGetRefusesAMissingVerdict(t *testing.T) {
	broker, fake := newFakeControlPlane(t, []credentialDoc{{
		EnvVar: "GITHUB_TOKEN", Hosts: []string{"api.github.com"}, Sentinel: "STABLE-1",
		Uses: []credentialUseDoc{{UseID: "use-1", Description: "open a PR"}},
	}})
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: newActivations()}

	_, err := b.Get(context.Background(), agentcreds.UseBody{UseID: "use-1", Command: []string{"gh"}})
	if !errors.Is(err, agentcreds.ErrInvalid) {
		t.Fatalf("get error = %v, want ErrInvalid for a body with no verdict", err)
	}
	if len(fake.verdictCalls) != 0 {
		t.Fatal("a rejected call still reached the control plane")
	}
}

// A control plane that cannot record the verdict must not mint a value
// anyway: the record is what makes the mint safe to have issued, not a
// courtesy alongside it.
func TestGetMintsNothingWhenRecordingTheVerdictFails(t *testing.T) {
	broker, fake := newFakeControlPlane(t, []credentialDoc{{
		EnvVar: "GITHUB_TOKEN", Hosts: []string{"api.github.com"}, Sentinel: "STABLE-1",
		Uses: []credentialUseDoc{{UseID: "use-1", Description: "open a PR"}},
	}})
	fake.verdictErr = errors.New("database unavailable")
	activations := newActivations()
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: activations}

	_, err := b.Get(context.Background(), agentcreds.UseBody{UseID: "use-1", Verdict: allowVerdict()})
	if err == nil {
		t.Fatal("get minted a value even though its verdict could not be recorded")
	}
}

// A denial never reaches Get (ADR 0079 §1's ordering mints nothing for a
// refusal), so ReportDenial is the only route it reaches the control plane
// by, and it must reach it with Volunteered set.
func TestReportDenialRecordsAVolunteeredVerdict(t *testing.T) {
	broker, fake := newFakeControlPlane(t, nil)
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: newActivations()}

	err := b.ReportDenial(context.Background(), agentcreds.DenialReport{
		UseID:   "use-1",
		Command: []string{"curl", "-X", "DELETE"},
		Verdict: agentcreds.Verdict{Allow: false, Reason: "broader than the approved use", Role: "judge", Prompt: "..."},
	})
	if err != nil {
		t.Fatalf("report denial: %v", err)
	}
	if len(fake.verdictCalls) != 1 || !fake.verdictCalls[0].Volunteered {
		t.Fatalf("verdict calls = %#v, want exactly one, volunteered", fake.verdictCalls)
	}
}

// A request by well-known ID goes out carrying the name, variable, and host the
// ID names, beside the ID the control plane checks them against; an ID the
// registry does not know is refused before anything is asked.
// The broker passes a well-known ask on as the agent sent it: the control
// plane fills in what the ID names and refuses what contradicts it, which it
// can only do if what the agent said reaches it unaltered.
func TestRequestPassesAWellKnownIDOnAsSent(t *testing.T) {
	broker, fake := newFakeControlPlane(t, nil)
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: newActivations()}

	if _, err := b.Request(context.Background(), agentcreds.RequestBody{ID: "com.github.api", EnvVar: "GITHUB_TOKEN", Uses: []agentcreds.RequestedUse{{Description: "open a PR"}}}); err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %d, want one", len(fake.requests))
	}
	got := fake.requests[0]
	if got.ID != "com.github.api" || got.Name != "" || got.EnvVar != "GITHUB_TOKEN" || got.Hosts != nil {
		t.Fatalf("request = %+v, want the ask as the agent sent it", got)
	}

	_, err := b.Request(context.Background(), agentcreds.RequestBody{ID: "com.example.nothing", Uses: []agentcreds.RequestedUse{{Description: "x"}}})
	if !errors.Is(err, agentcreds.ErrInvalid) || len(fake.requests) != 1 {
		t.Fatalf("err = %v, requests = %d; want an unknown ID refused as invalid before it is sent", err, len(fake.requests))
	}
}

// A use granted for several hosts is spent at any of them, and nowhere else,
// and list reports them all (ADR 26-10-02-393 §2).
func TestAUseGrantedForSeveralHostsIsSpentAtEach(t *testing.T) {
	broker, _ := newFakeControlPlane(t, []credentialDoc{{
		Name: "github", EnvVar: "GH_TOKEN", Sentinel: "STABLE-1",
		Hosts: []string{"api.github.com", "api.githubcopilot.com"},
		Uses:  []credentialUseDoc{{UseID: "use-1", Description: "run copilot"}},
	}})
	live := newActivations()
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: live}

	listed, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || strings.Join(listed[0].Hosts, ",") != "api.github.com,api.githubcopilot.com" {
		t.Fatalf("listed = %+v, want both hosts", listed)
	}

	out, err := b.Get(context.Background(), agentcreds.UseBody{UseID: "use-1", Command: []string{"copilot"}, Verdict: allowVerdict()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resolver := &secretResolver{activations: live}
	for host, want := range map[string]bool{
		"api.github.com":        true,
		"api.githubcopilot.com": true,
		// A sibling of a granted host is not beneath it.
		"api.individual.githubcopilot.com": false,
		"github.com":                       false,
		"evil.example.com":                 false,
	} {
		if _, ok := resolver.activation(proxy.SecretResolveRequest{ClientID: "sb-1", Sentinel: out.Value, Host: host}); ok != want {
			t.Errorf("activation at %s = %v, want %v", host, ok, want)
		}
	}
}

// An activation is pinned to the hosts its use was granted for. One minted
// from a credential that names no host covers nothing — never the wildcard an
// empty scope would otherwise read as.
func TestAnActivationIsNeverPinnedToNoHost(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  credentialDoc
		at   map[string]bool
	}{
		{"one host", credentialDoc{Hosts: []string{"api.github.com"}}, map[string]bool{"api.github.com": true, "api.githubcopilot.com": false}},
		{"no host", credentialDoc{}, map[string]bool{"api.github.com": false, "evil.example.com": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.doc
			doc.EnvVar, doc.Sentinel = "GH_TOKEN", "STABLE-1"
			doc.Uses = []credentialUseDoc{{UseID: "use-1", Description: "open a PR"}}
			broker, _ := newFakeControlPlane(t, []credentialDoc{doc})
			live := newActivations()
			b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: live}
			out, err := b.Get(context.Background(), agentcreds.UseBody{UseID: "use-1", Verdict: allowVerdict()})
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			resolver := &secretResolver{activations: live}
			for host, want := range tc.at {
				if _, ok := resolver.activation(proxy.SecretResolveRequest{ClientID: "sb-1", Sentinel: out.Value, Host: host}); ok != want {
					t.Errorf("activation at %s = %v, want %v", host, ok, want)
				}
			}
		})
	}
}

// An ask's hosts reach the control plane as the agent sent them.
func TestRequestPassesEveryHostOn(t *testing.T) {
	broker, fake := newFakeControlPlane(t, nil)
	b := &credentialBroker{sandboxID: "sb-1", controlPlan: broker, activations: newActivations()}
	if _, err := b.Request(context.Background(), agentcreds.RequestBody{
		Name: "github", EnvVar: "GH_TOKEN", Hosts: []string{"api.github.com", "api.githubcopilot.com"},
		Uses: []agentcreds.RequestedUse{{Description: "run copilot"}},
	}); err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(fake.requests) != 1 || strings.Join(fake.requests[0].Hosts, ",") != "api.github.com,api.githubcopilot.com" {
		t.Fatalf("relayed = %+v, want the hosts as sent", fake.requests)
	}
}
