package sandboxes

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/discobox-ai/discobox/server/internal/auth"
	poolagentauth "github.com/discobox-ai/discobox/server/internal/auth/poolagent"
)

func TestAuthorizeRequestedScopesRequiresHeldScopes(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		Type:   auth.PrincipalTypeUser,
		UserID: "user-1",
		Scopes: []string{
			poolagentauth.ScopeSandboxRead,
		},
	})

	if err := authorizeRequestedScopes(ctx, []string{poolagentauth.ScopeSandboxRead}); err != nil {
		t.Fatalf("authorize read scope: %v", err)
	}

	err := authorizeRequestedScopes(ctx, []string{poolagentauth.ScopeSandboxWrite})
	if err == nil {
		t.Fatal("authorize write scope succeeded, want failure")
	}
	var statusErr interface{ StatusCode() int }
	if !errors.As(err, &statusErr) || statusErr.StatusCode() != http.StatusForbidden {
		t.Fatalf("authorize write scope error = %v, want forbidden status", err)
	}
}

func TestAuthorizeRequestedScopesAllowsAllScope(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		Type:   auth.PrincipalTypeUser,
		UserID: "user-1",
		Scopes: []string{
			auth.ScopeAll,
		},
	})

	if err := authorizeRequestedScopes(ctx, []string{poolagentauth.ScopeSandboxRead, poolagentauth.ScopeSandboxWrite, poolagentauth.ScopeSandboxHTTP, poolagentauth.ScopeTerminalRead, poolagentauth.ScopeTerminalWrite, poolagentauth.ScopeExecRead, poolagentauth.ScopeExecWrite}); err != nil {
		t.Fatalf("authorize all scopes: %v", err)
	}
}

// A sandbox holds no scopes. The calls of its the sandbox role lets reach a
// sandbox are a push into the origin of a discobox it created (ADR
// 26-09-24-630 §2) and reading and typing into that discobox's terminals (ADR
// 26-10-01-397 §1), so those scopes are all it is allowed here: a route
// admitted by mistake still reaches no legacy terminal, tunnel, or sandbox HTTP.
func TestAuthorizeRequestedScopesAllowsASandboxOnlyAPushAndItsTerminals(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		Type: auth.PrincipalTypeSandbox, SandboxID: "sbx-lead", ProjectID: "proj-1", UserID: "user-1",
	})
	for _, scope := range []string{poolagentauth.ScopeSandboxWrite, poolagentauth.ScopeExecRead, poolagentauth.ScopeExecWrite} {
		if err := authorizeRequestedScopes(ctx, []string{scope}); err != nil {
			t.Fatalf("authorize a sandbox for %s: %v", scope, err)
		}
	}
	for _, scope := range []string{poolagentauth.ScopeSandboxRead, poolagentauth.ScopeSandboxHTTP, poolagentauth.ScopeTerminalRead,
		poolagentauth.ScopeTerminalWrite} {
		err := authorizeRequestedScopes(ctx, []string{scope})
		var statusErr interface{ StatusCode() int }
		if !errors.As(err, &statusErr) || statusErr.StatusCode() != http.StatusForbidden {
			t.Fatalf("authorize a sandbox for %s: err = %v, want forbidden", scope, err)
		}
	}
}

// A discobox never attaches to another, nor waits on an exec it created to use
// at once (ADR 26-10-01-397 §2): the waiting acquire refuses it before asking
// anything of the sandbox, whatever scopes it names.
func TestAwaitSandboxHTTPClientRefusesASandbox(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		Type: auth.PrincipalTypeSandbox, SandboxID: "sbx-lead", ProjectID: "proj-1", UserID: "user-1",
	})
	_, _, err := (&Service{}).AwaitSandboxHTTPClient(ctx, "proj-1", "sbx-worker", []string{poolagentauth.ScopeExecWrite, poolagentauth.ScopeExecRead})
	var statusErr interface{ StatusCode() int }
	if !errors.As(err, &statusErr) || statusErr.StatusCode() != http.StatusForbidden {
		t.Fatalf("await for a sandbox: err = %v, want forbidden", err)
	}
}
