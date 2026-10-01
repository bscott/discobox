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
// them, and the approval is made under that one: it is chosen once, checked
// again by its ID in the approval's transaction, and is the grant every later
// question about the approval — whether its uses fall within the delegation's,
// what the verdict records — is asked of.

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

// delegationFor chooses the delegation grant a discobox approves a request
// under, and the secret it answers with: the secret of a live delegation it
// holds that fits the request — the well-known credential it asked for, the
// secret the approver named, and a host that covers the one asked for. The
// approver does not choose among the project's secrets, only among what it was
// delegated; when that is more than one secret, it names which.
//
// Of that secret's delegations it takes the one that lets the grant last
// longest: the lifetime the approver named must fit within it, and one it did
// not name is fitted to it later (delegatedTTL). ttl is the lifetime named, and
// is read only when named.
func (s *Service) delegationFor(ctx context.Context, projectID, approverID string, req *model.SecretRequest, chosenID, host string, ttl int64, named bool) (*model.SecretGrant, *model.Secret, error) {
	delegations, err := s.store.ListLiveDelegationGrants(ctx, projectID, approverID)
	if err != nil {
		return nil, nil, err
	}
	if len(delegations) == 0 {
		return nil, nil, apperrors.NewStatusError(http.StatusForbidden,
			"this discobox holds no delegation grant, so it hands nothing on; ask a person for one with `discobox-access request --delegate`, or leave the request for a person")
	}
	// The secret the approver named is matched only against what it was
	// delegated — its ID, or a prefix of it, as IDs are matched elsewhere —
	// and never looked up among the project's: a secret it was not delegated
	// answers the same as one that does not exist.
	fits := map[string]*model.Secret{}
	bySecret := map[string][]*model.SecretGrant{}
	for i := range delegations {
		delegation := &delegations[i]
		if !hostscope.Covers(delegation.Host, host) {
			continue
		}
		if chosenID != "" && !strings.HasPrefix(delegation.SecretID, chosenID) {
			continue
		}
		secret, ok := fits[delegation.SecretID]
		if !ok {
			if secret, err = s.store.GetSecret(ctx, projectID, delegation.SecretID); err != nil {
				continue
			}
		}
		if req.WellKnownID != "" && secret.WellKnownID != req.WellKnownID {
			continue
		}
		fits[secret.ID] = secret
		bySecret[secret.ID] = append(bySecret[secret.ID], delegation)
	}
	switch len(fits) {
	case 0:
		what := "the credential this request asks for"
		if req.WellKnownID != "" {
			what = req.WellKnownID
		}
		return nil, nil, apperrors.NewStatusError(http.StatusForbidden, fmt.Sprintf(
			"no delegation grant this discobox holds hands on %s to %s; leave the request for a person", what, host))
	case 1:
	default:
		// By name or ID: a discobox's listing holds the secrets it was
		// delegated, so either resolves.
		names := make([]string, 0, len(fits))
		for _, secret := range fits {
			names = append(names, secret.Name+" ("+secret.ID+")")
		}
		sort.Strings(names)
		return nil, nil, apperrors.NewStatusError(http.StatusBadRequest, fmt.Sprintf(
			"this discobox was delegated more than one secret that fits; name which with --secret-id: %s", strings.Join(names, ", ")))
	}
	var secret *model.Secret
	for _, only := range fits {
		secret = only
	}
	delegation := longestLived(bySecret[secret.ID])
	if named && !outlastedBy(delegation, ttl, time.Now().UTC()) {
		return nil, nil, outlastsDelegation()
	}
	return delegation, secret, nil
}

// delegatedTTL checks, in the approval's transaction, that the delegation the
// approval was chosen under still bounds it — still the approver's, still
// live, still of this secret and covering the host — and returns the grant's
// lifetime within it: a lifetime nobody named is fitted to the delegation's
// remaining time, and one the approver named must fit or is refused. A
// delegation revoked or lapsed since it was chosen is not one this grant is
// made under, whatever else the approver holds.
func delegatedTTL(ctx context.Context, txStore *store.Store, projectID, approverID string, chosen *model.SecretGrant, secretID, host string, ttl int64, named bool) (int64, error) {
	gone := apperrors.NewStatusError(http.StatusForbidden,
		"the delegation grant this approval was made under is gone or no longer covers it; approve again, or leave the request for a person")
	delegation, err := txStore.GetSecretGrant(ctx, projectID, chosen.ID)
	if err != nil {
		return 0, gone
	}
	now := time.Now().UTC()
	if delegation.Purpose != model.SecretGrantPurposeDelegate || delegation.Scope != model.SecretGrantScopeSandbox ||
		delegation.ScopeKey != approverID || delegation.SecretID != secretID || !hostscope.Covers(delegation.Host, host) ||
		(delegation.ExpiresAt != nil && !delegation.ExpiresAt.After(now)) {
		return 0, gone
	}
	if delegation.ExpiresAt == nil {
		return ttl, nil
	}
	remaining := int64(delegation.ExpiresAt.Sub(now) / time.Second)
	if !named {
		return min(ttl, remaining), nil
	}
	if !outlastedBy(delegation, ttl, now) {
		return 0, outlastsDelegation()
	}
	return ttl, nil
}

// longestLived is the delegation that lets a grant last longest: one that never
// lapses, else the one that lapses last.
func longestLived(delegations []*model.SecretGrant) *model.SecretGrant {
	best := delegations[0]
	for _, delegation := range delegations[1:] {
		switch {
		case best.ExpiresAt == nil:
		case delegation.ExpiresAt == nil || delegation.ExpiresAt.After(*best.ExpiresAt):
			best = delegation
		}
	}
	return best
}

// outlastedBy reports whether a grant of ttl seconds — zero is forever — ends
// no later than the delegation it is made under.
func outlastedBy(delegation *model.SecretGrant, ttl int64, now time.Time) bool {
	if delegation.ExpiresAt == nil {
		return true
	}
	return ttl > 0 && !now.Add(time.Duration(ttl)*time.Second).After(*delegation.ExpiresAt)
}

func outlastsDelegation() error {
	return apperrors.NewStatusError(http.StatusForbidden,
		"a grant this discobox hands on may not outlast the delegation grant it is made under; leave the lifetime out to fit it, or grant it for less")
}
