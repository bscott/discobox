package secrets

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/discobox-ai/discobox/hostscope"
	"github.com/discobox-ai/discobox/server/internal/apperrors"
	"github.com/discobox-ai/discobox/server/internal/model"
	"github.com/discobox-ai/discobox/server/internal/store"
)

// A discobox approving a request hands on a credential, and may hand on only
// what it was delegated (ADR 26-09-30-782 §3). These are the facts of that
// bound — which credential, where it may go, for how long — held against the
// live delegation grants the approver holds. At least one must hold all of
// them; one is enough.

// refuseDelegatedApproval says why a discobox may not approve this request at
// all, whatever it was delegated, or nothing when it may try.
func refuseDelegatedApproval(req *model.SecretRequest) error {
	if req.Purpose == model.SecretGrantPurposeDelegate {
		return apperrors.NewStatusError(http.StatusForbidden,
			"a discobox never approves a request to delegate: handing on the power to hand on is a person's to approve")
	}
	// A request the proxy opened on meeting a sentinel it could not resolve
	// names no uses, and a grant with none authorizes everything sent to its
	// host — nothing a delegation's uses could be said to contain.
	if !req.FromProtocol() {
		return apperrors.NewStatusError(http.StatusForbidden,
			"a discobox approves only a request that names its uses; this one is a person's to answer")
	}
	return nil
}

// delegatedSecret is the secret a discobox answers a request with: the secret
// of a live delegation grant it holds that fits the request — the well-known
// credential it asked for, the secret the approver named, and a host that
// covers the one asked for. The approver does not choose among the project's
// secrets, only among what it was delegated; when that is more than one
// secret, it names which.
func (s *Service) delegatedSecret(ctx context.Context, projectID, approverID string, req *model.SecretRequest, chosenID, host string) (*model.Secret, error) {
	delegations, err := s.store.ListLiveDelegationGrants(ctx, projectID, approverID)
	if err != nil {
		return nil, err
	}
	if len(delegations) == 0 {
		return nil, apperrors.NewStatusError(http.StatusForbidden,
			"this discobox holds no delegation grant, so it hands nothing on; ask a person for one with `discobox-access request --delegate`, or leave the request for a person")
	}
	var chosen *model.Secret
	if chosenID != "" {
		if chosen, err = s.store.GetSecret(ctx, projectID, chosenID); err != nil {
			return nil, apperrors.NotFound(err, "secret not found")
		}
	}
	fits := map[string]*model.Secret{}
	for i := range delegations {
		delegation := &delegations[i]
		if !hostscope.Covers(delegation.Host, host) {
			continue
		}
		if chosen != nil && delegation.SecretID != chosen.ID {
			continue
		}
		secret, err := s.store.GetSecret(ctx, projectID, delegation.SecretID)
		if err != nil {
			continue
		}
		if req.WellKnownID != "" && secret.WellKnownID != req.WellKnownID {
			continue
		}
		fits[secret.ID] = secret
	}
	switch len(fits) {
	case 0:
		what := "the credential this request asks for"
		if req.WellKnownID != "" {
			what = req.WellKnownID
		}
		return nil, apperrors.NewStatusError(http.StatusForbidden, fmt.Sprintf(
			"no delegation grant this discobox holds hands on %s to %s; leave the request for a person", what, host))
	case 1:
		for _, secret := range fits {
			return secret, nil
		}
	}
	names := make([]string, 0, len(fits))
	for _, secret := range fits {
		names = append(names, secret.Name+" ("+secret.ID+")")
	}
	sort.Strings(names)
	return nil, apperrors.NewStatusError(http.StatusBadRequest, fmt.Sprintf(
		"this discobox was delegated more than one secret that fits; name which with --secret-id: %s", strings.Join(names, ", ")))
}

// delegatedTTL is the lifetime of a grant a discobox mints under a delegation
// grant it holds: one live grant of the same secret, covering the host, whose
// expiry the grant does not outlast. A lifetime nobody named is fitted to the
// delegation's remaining time; one the approver named must fit or is refused.
// It reads within the approval's transaction, so a delegation revoked or lapsed
// since the secret was chosen is not one this grant is made under.
func delegatedTTL(ctx context.Context, txStore *store.Store, projectID, approverID, secretID, host string, ttl int64, named bool) (int64, error) {
	delegations, err := txStore.ListLiveDelegationGrants(ctx, projectID, approverID)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	outlives := false
	for i := range delegations {
		delegation := &delegations[i]
		if delegation.SecretID != secretID || !hostscope.Covers(delegation.Host, host) {
			continue
		}
		if delegation.ExpiresAt == nil {
			return ttl, nil
		}
		remaining := int64(delegation.ExpiresAt.Sub(now) / time.Second)
		if remaining <= 0 {
			continue
		}
		if !named {
			return min(ttl, remaining), nil
		}
		if ttl > 0 && ttl <= remaining {
			return ttl, nil
		}
		outlives = true
	}
	if outlives {
		return 0, apperrors.NewStatusError(http.StatusForbidden,
			"a grant this discobox hands on may not outlast the delegation grant it is made under; grant it for less")
	}
	return 0, apperrors.NewStatusError(http.StatusForbidden,
		"the delegation grant this discobox held for this credential is gone or no longer covers it; leave the request for a person")
}
