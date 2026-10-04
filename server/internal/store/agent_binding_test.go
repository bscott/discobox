package store_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/discobox-ai/discobox/server/internal/apperrors"
	"github.com/discobox-ai/discobox/server/internal/model"
)

// collideAgentBindingCreates makes the next n creates of a sandbox secret fail
// as a unique index refuses a row a concurrent writer committed first. SQLite
// serializes writers, so the race itself only happens on Postgres; this is the
// error it ends in.
func collideAgentBindingCreates(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register("test:collide-agent-binding", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.SandboxSecret); ok && n > 0 {
			n--
			_ = tx.AddError(errors.New("UNIQUE constraint failed: sandbox_secrets.sandbox_id, sandbox_secrets.env_name, sandbox_secrets.agent_requested"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
}

// A first bind that loses the race to create the binding binds again, and the
// second attempt goes through.
func TestAFirstBindThatCollidesBindsAgain(t *testing.T) {
	ctx := context.Background()
	s, db := newTestStoreWithDB(t, nil)
	secret := &model.Secret{ProjectID: "project-1", Name: "github", Type: model.SecretTypeToken, EncryptedValue: []byte(`{"token":"ghp_abc"}`)}
	if err := s.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	collideAgentBindingCreates(t, db.Write, 1)

	binding, err := s.BindAgentSecret(ctx, "project-1", "sb-1", "GITHUB_TOKEN", secret)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	stored, err := s.FindAgentSandboxSecret(ctx, "project-1", "sb-1", "GITHUB_TOKEN")
	if err != nil {
		t.Fatalf("find binding: %v", err)
	}
	if stored.ID != binding.ID || stored.SecretID != secret.ID {
		t.Fatalf("binding = %#v, want the one bind returned, of %s", stored, secret.ID)
	}
}

// A bind that collides twice is answered with a 409, not the driver's
// constraint text as a 500.
func TestABindThatKeepsCollidingIsAConflict(t *testing.T) {
	ctx := context.Background()
	s, db := newTestStoreWithDB(t, nil)
	secret := &model.Secret{ProjectID: "project-1", Name: "github", Type: model.SecretTypeToken, EncryptedValue: []byte(`{"token":"ghp_abc"}`)}
	if err := s.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	collideAgentBindingCreates(t, db.Write, 2)

	_, err := s.BindAgentSecret(ctx, "project-1", "sb-1", "GITHUB_TOKEN", secret)
	var status apperrors.StatusError
	if !errors.As(err, &status) || status.Status != http.StatusConflict {
		t.Fatalf("bind = %v, want a 409", err)
	}
}
