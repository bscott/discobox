package sandboxcreate

import (
	"strings"
	"testing"
)

// --grant names a use of a secret for the new discobox. Repeating a secret,
// host, and variable gives that one grant several uses; anything that is not
// SECRET[@HOST]:ENV_VAR=USE is refused with the shape it should have had.
func TestGrantFlagsBecomeTheNewDiscoboxsGrants(t *testing.T) {
	grants, err := ParseGrants([]string{
		"github@github.com:GH_TOKEN=push a branch to org/repo",
		"npm:NPM_TOKEN=publish @org/pkg",
		"github@github.com:GH_TOKEN=open a pull request = the fix",
	})
	if err != nil {
		t.Fatalf("ParseGrants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("grants = %+v, want two", grants)
	}
	gh := grants[0]
	if gh.SecretId.Or("") != "github" || gh.EnvVar.Or("") != "GH_TOKEN" || strings.Join(gh.Hosts, ",") != "github.com" || len(gh.Uses) != 2 ||
		gh.Uses[0].Description != "push a branch to org/repo" || gh.Uses[1].Description != "open a pull request = the fix" {
		t.Fatalf("github grant = %+v, want both uses on one grant, in order", gh)
	}
	if npm := grants[1]; npm.SecretId.Or("") != "npm" || npm.Hosts != nil || len(npm.Uses) != 1 {
		t.Fatalf("npm grant = %+v, want the secret's own host", npm)
	}

	for _, bad := range []string{"github", "github:GH_TOKEN", ":GH_TOKEN=push", "github:=push", "github:GH_TOKEN= ", "=push", "@github.com=push"} {
		if _, err := ParseGrants([]string{bad}); err == nil || !strings.Contains(err.Error(), "ID[@HOST]=USE") {
			t.Fatalf("--grant %q: err = %v, want the shape it should have had", bad, err)
		}
	}
}

// Without a variable, a --grant names a well-known credential by its ID, which
// says its own secret and variable; the host may still be narrowed.
func TestAGrantFlagMayNameAWellKnownID(t *testing.T) {
	grants, err := ParseGrants([]string{
		"com.github.api=push a branch to org/repo",
		"com.github.api@api.github.com=read issues in org/repo",
		"com.github.api=open a pull request",
	})
	if err != nil {
		t.Fatalf("ParseGrants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("grants = %+v, want the site's grant and the narrower one", grants)
	}
	site, narrow := grants[0], grants[1]
	if site.WellKnownId.Or("") != "com.github.api" || site.SecretId.IsSet() || site.EnvVar.IsSet() || site.Hosts != nil || len(site.Uses) != 2 {
		t.Fatalf("site grant = %+v, want the ID alone, with both of its uses", site)
	}
	if narrow.WellKnownId.Or("") != "com.github.api" || strings.Join(narrow.Hosts, ",") != "api.github.com" || len(narrow.Uses) != 1 {
		t.Fatalf("narrow grant = %+v, want the ID narrowed to api.github.com", narrow)
	}
}

// A credential sent to more than one site names each, joined by commas
// (ADR 26-10-02-393).
func TestAGrantMayNameSeveralHosts(t *testing.T) {
	grants, err := ParseGrants([]string{"com.github.api@api.github.com, githubcopilot.com=run copilot against org/repo"})
	if err != nil {
		t.Fatalf("ParseGrants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("grants = %+v, want one", grants)
	}
	if g := grants[0]; strings.Join(g.Hosts, ",") != "api.github.com,githubcopilot.com" {
		t.Fatalf("grant hosts = %q; want api.github.com, then githubcopilot.com", g.Hosts)
	}
}

// The same hosts in another order or spacing are one grant, and naming none
// leaves the list out for the server's default.
func TestGrantHostsAreASet(t *testing.T) {
	grants, err := ParseGrants([]string{
		"com.github.api@api.github.com,githubcopilot.com=run copilot",
		"com.github.api@ githubcopilot.com , API.github.com=open a pull request",
		"github@github.com:GH_TOKEN=push a branch",
	})
	if err != nil {
		t.Fatalf("ParseGrants: %v", err)
	}
	if len(grants) != 2 || len(grants[0].Uses) != 2 {
		t.Fatalf("grants = %+v, want the two spellings of one host set as one grant with two uses", grants)
	}
	if got := strings.Join(grants[1].Hosts, ","); got != "github.com" {
		t.Fatalf("a one-host grant carries hosts %q, want github.com", got)
	}
}
