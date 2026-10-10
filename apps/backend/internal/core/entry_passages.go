package core

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/passage"
	"sort"
)

// EntryPassages honors approved entry boundaries while preserving exact offsets.
// It is used only when preparing unpublished text; published chunks never move.
func EntryPassages(ctx context.Context, tx pgx.Tx, text string, revision uuid.UUID, pageID, blockID *uuid.UUID) ([]passage.Span, error) {
	rows, err := tx.Query(ctx, `SELECT l.start_character,l.end_character FROM structured_entry_locations l JOIN structured_entries e ON e.id=l.entry_id WHERE e.processing_revision_id=$1 AND e.kind='materia_medica' AND e.review_status IN ('accepted','corrected') AND (l.page_id=$2 OR l.document_block_id=$3)`, revision, pageID, blockID)
	if err != nil {
		return nil, err
	}
	r := []rune(text)
	bounds := []int{0, len(r)}
	for rows.Next() {
		var start, end int
		if err = rows.Scan(&start, &end); err != nil {
			rows.Close()
			return nil, err
		}
		if start < 0 || end > len(r) || end <= start {
			rows.Close()
			return nil, fmt.Errorf("entry boundary outside current text")
		}
		bounds = append(bounds, start, end)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sort.Ints(bounds)
	out := []passage.Span{}
	for i := 1; i < len(bounds); i++ {
		if bounds[i] == bounds[i-1] {
			continue
		}
		for _, span := range passage.Split(string(r[bounds[i-1]:bounds[i]]), 1800) {
			span.Start += bounds[i-1]
			span.End += bounds[i-1]
			out = append(out, span)
		}
	}
	return out, nil
}
