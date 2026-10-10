package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// All browser routes share the publication, revision, collection and current
// embedding-configuration gates. Search never reads the draft review APIs.
const referenceEligible = `WITH eligible AS (
 SELECT e.*,s.title,s.author,s.edition,s.repository,s.document_format
 FROM eligible_structured_entries e JOIN sources s ON s.id=e.source_id
 JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
 JOIN embedding_configs ec ON ec.id=ir.embedding_config_id
 WHERE s.removed_at IS NULL
 AND ec.model_id=$1 AND ec.model_revision=$2 AND ec.dimensions=$3
`

const repertoryEligible = referenceEligible + ` AND e.kind='repertory_rubric'
), associations AS (
 SELECT rr.* FROM eligible_rubric_remedies rr JOIN eligible e ON e.id=rr.rubric_id
) `

func (a *API) referenceBrowserConfig(w http.ResponseWriter) ([]any, bool) {
	if a.Model == nil {
		fail(w, 503, "reference browsing requires an active embedding configuration")
		return nil, false
	}
	c := a.Model.Config
	return []any{c.EmbeddingModel, c.EmbeddingRevision, c.Dimensions}, true
}

func (a *API) repertoryCatalog(w http.ResponseWriter, r *http.Request) {
	args, ok := a.referenceBrowserConfig(w)
	if !ok {
		return
	}
	var data json.RawMessage
	err := a.Store.DB.QueryRow(r.Context(), repertoryEligible+`SELECT jsonb_build_object(
 'sources',coalesce((SELECT jsonb_agg(x ORDER BY title,edition,id) FROM
 (SELECT DISTINCT source_id AS id,title,author,edition,repository FROM eligible) x),'[]'::jsonb),
 'chapters',coalesce((SELECT jsonb_agg(x ORDER BY chapter,source_id) FROM
 (SELECT DISTINCT source_id,full_path[1] AS chapter FROM eligible) x),'[]'::jsonb),
 'remedies',coalesce((SELECT jsonb_agg(x ORDER BY canonical_name,preparation_key) FROM
 (SELECT DISTINCT m.id,m.canonical_name,m.preparation_key FROM associations rr JOIN remedies m ON m.id=rr.remedy_id) x),'[]'::jsonb))`, args...).Scan(&data)
	if err != nil {
		fail(w, 500, "could not load repertory catalog")
		return
	}
	write(w, 200, data)
}

type repertoryQuery struct {
	Text, Chapter, Parent, Scope string
	Sources                      []uuid.UUID
	Remedy                       *uuid.UUID
	Offset                       int
}

func parseRepertoryQuery(r *http.Request) (repertoryQuery, bool) {
	v := r.URL.Query()
	q := repertoryQuery{Text: strings.TrimSpace(v.Get("q")), Chapter: v.Get("chapter"), Parent: v.Get("parent"), Scope: v.Get("scope"), Sources: []uuid.UUID{}}
	if len(q.Text) > 300 || len(q.Chapter) > 500 || (q.Scope != "" && q.Scope != "all" && q.Scope != "selected") {
		return q, false
	}
	for _, raw := range v["source_id"] {
		id, err := uuid.Parse(raw)
		if err != nil {
			return q, false
		}
		q.Sources = append(q.Sources, id)
	}
	if len(q.Sources) > 100 {
		return q, false
	}
	if raw := v.Get("remedy_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return q, false
		}
		q.Remedy = &id
	}
	if q.Parent != "" && q.Parent != "root" {
		if _, err := uuid.Parse(q.Parent); err != nil {
			return q, false
		}
	}
	if raw := v.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 100000 {
			return q, false
		}
		q.Offset = n
	}
	return q, true
}

func (a *API) searchRepertory(w http.ResponseWriter, r *http.Request) {
	q, ok := parseRepertoryQuery(r)
	if !ok {
		fail(w, 400, "invalid repertory search filters")
		return
	}
	args, ok := a.referenceBrowserConfig(w)
	if !ok {
		return
	}
	args = append(args, q.Text, q.Sources, q.Scope == "selected", q.Chapter, q.Parent, q.Remedy, q.Offset)
	var data json.RawMessage
	err := a.Store.DB.QueryRow(r.Context(), repertoryEligible+`, matched AS (
 SELECT e.id,e.source_id,e.processing_revision_id,e.parent_id,e.heading,e.full_path,e.title,e.author,e.edition,e.repository,
 (WITH RECURSIVE parents AS (
 SELECT a.id,a.parent_id,a.heading,1 AS depth,ARRAY[a.id] AS visited FROM eligible a WHERE a.id=e.parent_id
 UNION ALL SELECT a.id,a.parent_id,a.heading,p.depth+1,p.visited||a.id FROM eligible a JOIN parents p ON a.id=p.parent_id
 WHERE NOT a.id=ANY(p.visited) AND p.depth<100)
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'heading',heading) ORDER BY depth DESC),'[]'::jsonb) FROM parents) AS ancestors,
 (SELECT count(DISTINCT rr.remedy_id) FROM associations rr WHERE rr.rubric_id=e.id) AS verified_remedy_count,
 EXISTS(SELECT 1 FROM eligible child WHERE child.parent_id=e.id) AS has_children,
 CASE WHEN lower(e.heading)=lower($4) THEN 3
 WHEN lower(array_to_string(e.full_path,' '))=lower($4) THEN 2
 WHEN $4<>'' AND strpos(lower(array_to_string(e.full_path,' ')),lower($4))>0 THEN 1 ELSE 0 END AS exact_rank,
 ts_rank_cd(to_tsvector('simple',array_to_string(e.full_path,' ')),plainto_tsquery('simple',$4)) AS lexical_rank
 FROM eligible e WHERE
 (NOT $6::boolean AND cardinality($5::uuid[])=0 OR e.source_id=ANY($5::uuid[]))
 AND ($7='' OR e.full_path[1]=$7)
 AND ($8='' OR ($8='root' AND e.parent_id IS NULL) OR e.parent_id::text=$8)
 AND ($9::uuid IS NULL OR EXISTS(SELECT 1 FROM associations rr WHERE rr.rubric_id=e.id AND rr.remedy_id=$9))
 AND ($4='' OR lower(e.heading)=lower($4) OR to_tsvector('simple',array_to_string(e.full_path,' ')) @@ plainto_tsquery('simple',$4))
), page AS (SELECT * FROM matched ORDER BY exact_rank DESC,lexical_rank DESC,full_path,source_id,id LIMIT 50 OFFSET $10)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM matched),'offset',$10::int,'limit',50,
 'items',coalesce((SELECT jsonb_agg(page ORDER BY exact_rank DESC,lexical_rank DESC,full_path,source_id,id) FROM page),'[]'::jsonb),
 'coverage','Reviewed eligible records only; counts do not establish complete coverage of a book.')`, args...).Scan(&data)
	if err != nil {
		fail(w, 500, "could not search verified rubrics")
		return
	}
	write(w, 200, data)
}

func (a *API) repertoryRubric(w http.ResponseWriter, r *http.Request) {
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
	err := a.Store.DB.QueryRow(r.Context(), repertoryEligible+`SELECT jsonb_build_object(
 'id',e.id,'source_id',e.source_id,'processing_revision_id',e.processing_revision_id,
 'title',e.title,'edition',e.edition,'full_path',e.full_path,
 'locations',coalesce((SELECT jsonb_agg(jsonb_build_object('exact_text',l.exact_text,'start_character',l.start_character,'end_character',l.end_character,
 'page_id',l.page_id,'document_block_id',l.document_block_id,'scan',p.pdf_page_index+1,
 'original_url',CASE WHEN l.page_id IS NOT NULL THEN '/api/v1/sources/'||e.source_id||'/pdf#page='||(p.pdf_page_index+1)
 ELSE '/api/v1/sources/'||e.source_id||'/document/'||e.processing_revision_id||'/reader' END)
 ORDER BY p.pdf_page_index,b.block_index,l.start_character)
 FROM structured_entry_locations l LEFT JOIN pages p ON p.id=l.page_id LEFT JOIN document_blocks b ON b.id=l.document_block_id WHERE l.entry_id=e.id),'[]'::jsonb),
 'remedies',coalesce((SELECT jsonb_agg(jsonb_build_object('id',rr.id,'remedy_id',rr.remedy_id,'canonical_name',m.canonical_name,
 'preparation_key',m.preparation_key,'source_notation',rr.source_notation,'source_remedy_spelling',rr.source_remedy_spelling,
 'grade',rr.grade,'grade_scheme',rr.grade_scheme,'review_status',rr.review_status,
 'locations',coalesce((SELECT jsonb_agg(jsonb_build_object('exact_text',l.exact_text,'start_character',l.start_character,'end_character',l.end_character,
 'page_id',l.page_id,'document_block_id',l.document_block_id,
 'original_url',CASE WHEN l.page_id IS NOT NULL THEN '/api/v1/sources/'||e.source_id||'/pdf#page='||(p.pdf_page_index+1)
 ELSE '/api/v1/sources/'||e.source_id||'/document/'||e.processing_revision_id||'/reader' END)
 ORDER BY p.pdf_page_index,b.block_index,l.start_character)
 FROM rubric_remedy_locations l LEFT JOIN pages p ON p.id=l.page_id LEFT JOIN document_blocks b ON b.id=l.document_block_id WHERE l.association_id=rr.id),'[]'::jsonb))
 ORDER BY m.canonical_name,m.preparation_key,rr.id) FROM associations rr JOIN remedies m ON m.id=rr.remedy_id WHERE rr.rubric_id=e.id),'[]'::jsonb))
 FROM eligible e WHERE e.id=$4`, args...).Scan(&data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "verified rubric is unavailable")
		} else {
			fail(w, 500, "could not read verified rubric")
		}
		return
	}
	write(w, 200, data)
}
