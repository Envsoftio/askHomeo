WITH labels AS (
 SELECT DISTINCT a.id AS answer_id, (m.match)[1]::int AS number
 FROM answers a
 CROSS JOIN LATERAL regexp_matches(a.answer_text, '\[E([1-9]|10)\]', 'g') AS m(match)
), ranked AS (
 SELECT answer_id, 'E'||number AS label,
        row_number() OVER (PARTITION BY answer_id ORDER BY number) AS ordinal
 FROM labels
)
UPDATE answer_citations ac SET evidence_label=r.label
FROM ranked r
WHERE ac.answer_id=r.answer_id AND ac.ordinal=r.ordinal;
CREATE UNIQUE INDEX answer_citations_label_idx ON answer_citations(answer_id,evidence_label);
