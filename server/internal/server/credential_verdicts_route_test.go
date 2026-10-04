package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/discobox-ai/discobox/server/internal/model"
	"github.com/discobox-ai/discobox/server/internal/store"
)

// The verdict trail read back by a project member through
// list-credential-verdicts, after the sandbox it describes is gone. Every layer
// between runs for real — ogen's query decoding of the tri-state allow and the
// date-time since, the project authorizer, the handler, the store.
func TestCredentialVerdictsReadBackAfterTheirSandboxIsGone(t *testing.T) {
	skipWithoutDocker(t)
	ctx := context.Background()
	db := newAppTestDB(ctx, t)
	router := newTestApp(ctx, t, db)
	projectID, _ := seedCredentialRoutePool(ctx, t, db.Write, router)
	before := time.Now().UTC().Add(-time.Minute)

	appStore := store.New(db.Write, db.Read)
	for _, row := range []*model.CredentialVerdict{
		{Command: []string{"gh", "pr", "create"}, Allow: true, Reason: "matches the approved use", LatencyMS: 400},
		{Command: []string{"gh", "repo", "delete"}, Allow: false, Reason: "not what was approved", LatencyMS: 350},
	} {
		row.ProjectID, row.SandboxID, row.UseID = projectID, routeTestSandboxID, "use_issued"
		row.Kind, row.Origin, row.Round, row.Prompt = model.CredentialVerdictKindCommand, model.CredentialVerdictOriginJudge, 1, "{}"
		if err := appStore.CreateCredentialVerdict(ctx, row); err != nil {
			t.Fatalf("record verdict: %v", err)
		}
	}

	// Purge the sandbox out from under its trail.
	if err := db.Write.WithContext(ctx).Delete(&model.Sandbox{ID: routeTestSandboxID}).Error; err != nil {
		t.Fatalf("delete sandbox: %v", err)
	}

	list := func(t *testing.T, query url.Values) []model.CredentialVerdict {
		t.Helper()
		resp := callRoute(t, router, http.MethodGet, "/projects/"+projectID+"/credential-verdicts?"+query.Encode(), "", "")
		if resp.Code != http.StatusOK {
			t.Fatalf("list %v status = %d, body = %s", query, resp.Code, resp.Body.String())
		}
		var body struct {
			CredentialVerdicts []model.CredentialVerdict `json:"credentialVerdicts"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.CredentialVerdicts
	}

	all := list(t, url.Values{"sandboxId": {routeTestSandboxID}})
	if len(all) != 2 {
		t.Fatalf("got %d verdicts for the purged sandbox, want 2", len(all))
	}

	// Bounds are sent with offsets at the two ends of the range, the way a
	// client in another zone sends them. SQLite compares times as text, so a
	// bound is only read as an instant if the store puts it in the zone the
	// rows are in. Earliest-possible and latest-possible offsets make any
	// server zone disagree with them, so this fails without that step wherever
	// the test runs.
	farEast := time.FixedZone("UTC+14", 14*60*60)
	farWest := time.FixedZone("UTC-12", -12*60*60)
	denials := list(t, url.Values{"allow": {"false"}, "since": {before.In(farEast).Format(time.RFC3339)}})
	if len(denials) != 1 || denials[0].Allow || denials[0].Reason != "not what was approved" {
		t.Fatalf("denials = %+v, want the one denial", denials)
	}

	if future := list(t, url.Values{"since": {time.Now().Add(time.Hour).In(farWest).Format(time.RFC3339)}}); len(future) != 0 {
		t.Fatalf("since an hour ahead returned %d verdicts, want none", len(future))
	}
}

func TestCredentialVerdictsRejectsAnOutOfRangeLimit(t *testing.T) {
	skipWithoutDocker(t)
	ctx := context.Background()
	db := newAppTestDB(ctx, t)
	router := newTestApp(ctx, t, db)
	projectID := defaultProjectID(ctx, t, router)
	for _, limit := range []string{"0", "1001"} {
		resp := callRoute(t, router, http.MethodGet, "/projects/"+projectID+"/credential-verdicts?limit="+limit, "", "")
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("limit=%s status = %d, want 400", limit, resp.Code)
		}
	}
}
