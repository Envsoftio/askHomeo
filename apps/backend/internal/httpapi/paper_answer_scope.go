package httpapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var errPaperSearchScope = errors.New("selected papers must belong to your saved search and have linked sources")

// A paper search never widens research scope. Every requested source must be
// linked to a reference saved from this exact, user-owned search observation.
func (a *API) validatePaperSearchScope(ctx context.Context, searchID *uuid.UUID, sourceIDs []uuid.UUID) error {
	if searchID == nil {
		return nil
	}
	if *searchID == uuid.Nil || len(sourceIDs) == 0 {
		return errPaperSearchScope
	}
	var matched int
	err := a.Store.DB.QueryRow(ctx, `SELECT count(DISTINCT sw.source_id) FROM paper_searches ps
	 JOIN scholarly_observations so ON so.search_id=ps.id
	 JOIN scholarly_works sw ON sw.id=so.work_id
	 JOIN paper_saved_references pr ON pr.work_id=sw.id AND pr.owner_principal_id=ps.owner_principal_id
	 WHERE ps.id=$1 AND ps.owner_principal_id=$2 AND sw.source_id=ANY($3::uuid[])`, *searchID, requestPrincipal(ctx).ID, sourceIDs).Scan(&matched)
	if err != nil || matched != len(sourceIDs) {
		return errPaperSearchScope
	}
	return nil
}

func paperSearchIDFromContext(ctx context.Context) *uuid.UUID {
	id, ok := ctx.Value(paperSearchKey{}).(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}

type paperSearchKey struct{}
