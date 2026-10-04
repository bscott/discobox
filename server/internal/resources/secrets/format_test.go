package secrets_test

import (
	"net/http"
	"strings"
	"testing"

	apigen "github.com/discobox-ai/discobox/api/gen"
	"github.com/discobox-ai/discobox/server/internal/services"
)

// A format a person sets is the one sentinels are minted from, and it outlives
// the value it was set against; clearing it goes back to reading the value.
func TestSecretFormatSetOutlivesTheValue(t *testing.T) {
	svc := newTestService(t)
	ctx := testPrincipalContext()
	apiKey := "sk-ant-api03-" + strings.Repeat("a", 95)

	sec, err := svc.CreateSecret(ctx, "project-1", services.CreateSecretBody{
		Name:   "claude",
		Type:   apigen.CreateSecretBodyTypeToken,
		Value:  apigen.SecretValue{Token: apigen.NewOptString(apiKey)},
		Format: apigen.NewOptString("sk-custom-{hex:40}"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sec.Format != "sk-custom-{hex:40}" || !sec.FormatSet {
		t.Fatalf("created format = %q set=%t, want the one given", sec.Format, sec.FormatSet)
	}

	sec, err = svc.UpdateSecret(ctx, "project-1", sec.ID, services.UpdateSecretBody{
		Value: apigen.NewOptSecretValue(apigen.SecretValue{Token: apigen.NewOptString("sk-ant-api03-" + strings.Repeat("b", 95))}),
	})
	if err != nil {
		t.Fatalf("replace value: %v", err)
	}
	if sec.Format != "sk-custom-{hex:40}" || !sec.FormatSet {
		t.Fatalf("format after a new value = %q set=%t, want it kept", sec.Format, sec.FormatSet)
	}

	sec, err = svc.UpdateSecret(ctx, "project-1", sec.ID, services.UpdateSecretBody{Format: apigen.NewOptString("")})
	if err != nil {
		t.Fatalf("clear format: %v", err)
	}
	if !strings.HasPrefix(sec.Format, "sk-ant-api03-") || sec.FormatSet {
		t.Fatalf("cleared format = %q set=%t, want it read from the value", sec.Format, sec.FormatSet)
	}
}

// A template that does not parse is refused rather than stored, where the
// minter would have quietly fallen back to the default shape.
func TestSecretFormatMustParse(t *testing.T) {
	svc := newTestService(t)
	ctx := testPrincipalContext()
	for _, format := range []string{"sk-{nope:4}", "sk-{hex}", "{alnum:5000}"} {
		_, err := svc.CreateSecret(ctx, "project-1", services.CreateSecretBody{
			Name:   "bad",
			Type:   apigen.CreateSecretBodyTypeToken,
			Value:  apigen.SecretValue{Token: apigen.NewOptString("x")},
			Format: apigen.NewOptString(format),
		})
		if statusOf(err) != http.StatusBadRequest {
			t.Fatalf("format %q err = %v, want 400", format, err)
		}
	}
}
