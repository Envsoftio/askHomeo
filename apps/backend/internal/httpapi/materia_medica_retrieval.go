package httpapi

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

const mmRetrievalSQL = `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url,cat.categories,p.format,s.evidence_category
 FROM chunks c JOIN sources s ON s.id=c.source_id
 JOIN evidence_locations p ON p.chunk_id=c.id JOIN evidence_categories cat ON cat.chunk_id=c.id
 JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
 JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id
 WHERE ir.embedding_config_id=$1 AND ir.status='ready' AND s.removed_at IS NULL
 AND ($3='' OR (s.source_key LIKE $3||'-%' OR s.author ILIKE '%'||$3||'%' OR s.title ILIKE '%'||$3||'%'))
 AND (cardinality($4::uuid[])=0 OR s.id=ANY($4::uuid[])) AND (cardinality($5::text[])=0 OR cat.categories && $5::text[])
 AND EXISTS(SELECT 1 FROM eligible_mm_chunks mm WHERE mm.chunk_id=c.id AND (strpos(' '||btrim(regexp_replace(lower($2::text),'[^[:alnum:]]+',' ','g'))||' ',' '||btrim(regexp_replace(lower(mm.canonical_name),'[^[:alnum:]]+',' ','g'))||' ')>0
 OR strpos(' '||btrim(regexp_replace(lower($2::text),'[^[:alnum:]]+',' ','g'))||' ',' '||btrim(regexp_replace(lower(mm.source_remedy_spelling),'[^[:alnum:]]+',' ','g'))||' ')>0)
  )
 ORDER BY ts_rank_cd(c.search_vector,plainto_tsquery('english',$2)) DESC,p.pdf_page_index,c.chunk_index,c.id LIMIT 40`

func (a *API) attachMMContext(ctx context.Context, hits map[uuid.UUID]*hit) error {
	ids := make([]uuid.UUID, 0, len(hits))
	for id := range hits {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := a.Store.DB.Query(ctx, `SELECT chunk_id,entry_id,canonical_name,preparation_key,source_remedy_spelling FROM eligible_mm_chunks WHERE chunk_id=ANY($1::uuid[]) ORDER BY chunk_id,entry_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var chunk, entry uuid.UUID
		var name, prep, spelling string
		if err = rows.Scan(&chunk, &entry, &name, &prep, &spelling); err != nil {
			return err
		}
		if h := hits[chunk]; h != nil {
			h.RemedyContext += fmt.Sprintf("Reviewed entry %s: %s; preparation: %s; source spelling: %s. ", entry, name, prep, spelling)
		}
	}
	return rows.Err()
}
func promptPassage(h hit) string {
	if h.RemedyContext == "" {
		return h.Text
	}
	return "Verified source-entry identity (context only, not quotation): " + h.RemedyContext + "\nExact source passage:\n" + h.Text
}
