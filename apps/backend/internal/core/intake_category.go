package core

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IntakeCategory records an explicit administrator choice in the same transaction
// as the imported source, before a worker can suggest a replacement category.
type IntakeCategory struct {
	Categories []string
	ActorID    uuid.UUID
}

func recordIntakeCategory(ctx context.Context, tx pgx.Tx, id uuid.UUID, choice IntakeCategory) error {
	if len(choice.Categories) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE sources SET literature_categories=$2,literature_category_origin='manual' WHERE id=$1`, id, choice.Categories)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,actor_principal_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale)
 SELECT $2,id,current_revision_id,$3,ARRAY['unclassified']::text[],$4,'unknown',evidence_category,'manual','Administrator selected literature category at intake' FROM sources WHERE id=$1`, id, uuid.New(), choice.ActorID, choice.Categories)
	return err
}
