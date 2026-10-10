package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

const mmBrowserEligible = referenceEligible + ` AND e.kind='materia_medica' AND e.remedy_id IS NOT NULL
 AND EXISTS(SELECT 1 FROM eligible_mm_chunks mm WHERE mm.entry_id=e.id)
) `

func (a *API) mmCatalog(w http.ResponseWriter, r *http.Request) {
	args, ok := a.referenceBrowserConfig(w)
	if !ok {
		return
	}
	var data json.RawMessage
	err := a.Store.DB.QueryRow(r.Context(), mmBrowserEligible+`SELECT jsonb_build_object(
 'sources',coalesce((SELECT jsonb_agg(x ORDER BY title,edition,id) FROM (SELECT DISTINCT source_id AS id,title,author,edition,repository FROM eligible) x),'[]'::jsonb),
 'remedies',coalesce((SELECT jsonb_agg(x ORDER BY canonical_name,preparation_key,id) FROM (SELECT DISTINCT m.id,m.canonical_name,m.preparation_key FROM eligible e JOIN remedies m ON m.id=e.remedy_id) x),'[]'::jsonb))`, args...).Scan(&data)
	if err != nil {
		fail(w, 500, "could not load materia medica catalog")
		return
	}
	write(w, 200, data)
}

func (a *API) searchMM(w http.ResponseWriter, r *http.Request) {
	q, ok := parseRepertoryQuery(r)
	if !ok || q.Parent != "" || q.Chapter != "" {
		fail(w, 400, "invalid materia medica filters")
		return
	}
	args, ok := a.referenceBrowserConfig(w)
	if !ok {
		return
	}
	args = append(args, q.Text, q.Sources, q.Scope == "selected", q.Remedy, q.Offset)
	var data json.RawMessage
	err := a.Store.DB.QueryRow(r.Context(), mmBrowserEligible+`, matched AS (
 SELECT e.id,e.source_id,e.processing_revision_id,e.heading,e.subsection,e.source_remedy_spelling,e.remedy_id,
 e.title,e.author,e.edition,e.repository,e.document_format,m.canonical_name,m.preparation_key,
 (SELECT count(*) FROM structured_entry_locations l WHERE l.entry_id=e.id) AS location_count,
 CASE WHEN lower(m.canonical_name)=lower($4) OR lower(e.source_remedy_spelling)=lower($4) THEN 1 ELSE 0 END AS exact_rank
 FROM eligible e JOIN remedies m ON m.id=e.remedy_id
 WHERE (NOT $6::boolean AND cardinality($5::uuid[])=0 OR e.source_id=ANY($5::uuid[]))
 AND ($7::uuid IS NULL OR e.remedy_id=$7)
 AND ($4='' OR strpos(lower(m.canonical_name),lower($4))>0 OR strpos(lower(e.source_remedy_spelling),lower($4))>0
 OR to_tsvector('simple',e.heading||' '||e.subsection||' '||m.canonical_name) @@ plainto_tsquery('simple',$4))
), page AS (SELECT * FROM matched ORDER BY exact_rank DESC,canonical_name,preparation_key,title,source_id,id LIMIT 50 OFFSET $8)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM matched),'offset',$8::int,'limit',50,
 'items',coalesce((SELECT jsonb_agg(page ORDER BY exact_rank DESC,canonical_name,preparation_key,title,source_id,id) FROM page),'[]'::jsonb))`, args...).Scan(&data)
	if err != nil {
		fail(w, 500, "could not search materia medica entries")
		return
	}
	write(w, 200, data)
}

func (a *API) mmEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	args, ok := a.referenceBrowserConfig(w)
	if !ok {
		return
	}
	args = append(args, id)
	var data json.RawMessage
	err := a.Store.DB.QueryRow(r.Context(), mmBrowserEligible+`SELECT jsonb_build_object('id',e.id,'source_id',e.source_id,
 'heading',e.heading,'canonical_name',m.canonical_name,'preparation_key',m.preparation_key,'title',e.title,'author',e.author,'edition',e.edition,'repository',e.repository,
 'locations',coalesce((SELECT jsonb_agg(jsonb_build_object('exact_text',l.exact_text,'start_character',l.start_character,'end_character',l.end_character,
 'label',coalesce(nullif(p.printed_label,''),nullif(b.heading,''),b.section_key,''),'position',coalesce(p.pdf_page_index,b.block_index)+1,
 'kind',CASE WHEN l.page_id IS NOT NULL THEN 'page' ELSE 'block' END,
 'original_url',CASE WHEN l.page_id IS NOT NULL THEN '/api/v1/sources/'||e.source_id||'/pdf#page='||(p.pdf_page_index+1)
 ELSE '/api/v1/sources/'||e.source_id||'/document/'||e.processing_revision_id||'/reader' END)
 ORDER BY p.pdf_page_index,b.block_index,l.start_character)
 FROM structured_entry_locations l LEFT JOIN pages p ON p.id=l.page_id LEFT JOIN document_blocks b ON b.id=l.document_block_id WHERE l.entry_id=e.id),'[]'::jsonb))
 FROM eligible e JOIN remedies m ON m.id=e.remedy_id WHERE e.id=$4`, args...).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "verified materia medica entry is unavailable")
		return
	}
	if err != nil {
		fail(w, 500, "could not read materia medica entry")
		return
	}
	write(w, 200, data)
}
