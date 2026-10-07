package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type inputCase struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	Question     string   `json:"question"`
	SourceTitles []string `json:"source_titles"`
	HeldOut      bool     `json:"held_out"`
}
type candidate struct {
	ChunkID      uuid.UUID `json:"chunk_id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	ScanPosition int       `json:"scan_position"`
	PrintedPage  string    `json:"printed_page"`
	ImageURL     string    `json:"image_url"`
	Passage      string    `json:"passage"`
}
type reviewCase struct {
	inputCase
	Candidates     []candidate `json:"candidate_passages"`
	Reviewer       string      `json:"reviewer"`
	ReviewedAt     string      `json:"reviewed_at"`
	GoldChunkIDs   []uuid.UUID `json:"gold_chunk_ids"`
	ExpectedPoints []string    `json:"expected_points"`
}

var terms = regexp.MustCompile(`[A-Za-z]{3,}`)
var stop = map[string]bool{"what": true, "which": true, "does": true, "about": true, "describe": true, "describes": true, "compare": true, "each": true, "their": true, "them": true, "with": true, "from": true, "the": true, "and": true, "for": true, "who": true, "how": true, "nash": true, "farrington": true, "book": true, "books": true, "prepared": true, "source": true, "selected": true, "homoeopathic": true, "homeopathic": true, "materia": true, "medica": true}

func queryTerms(question string) string {
	seen := map[string]bool{}
	words := []string{}
	for _, raw := range terms.FindAllString(strings.ToLower(question), -1) {
		if stop[raw] || seen[raw] {
			continue
		}
		seen[raw] = true
		words = append(words, raw)
		if len(words) == 6 {
			break
		}
	}
	return strings.Join(words, " | ")
}

func main() {
	input := flag.String("input", "evaluation/questions.draft.json", "question set")
	output := flag.String("output", "evaluation/review_worksheet.json", "review packet")
	markdown := flag.String("markdown", "evaluation/review_worksheet.md", "readable review packet")
	flag.Parse()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		log.Fatal(err)
	}
	var dataset struct {
		Cases []inputCase `json:"cases"`
	}
	if err = json.Unmarshal(data, &dataset); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	packet := struct {
		Note  string       `json:"note"`
		Cases []reviewCase `json:"cases"`
	}{Note: "Candidate passages are lexical suggestions, not gold evidence. Check each original scan and the full source context before selecting gold_chunk_ids and expected_points.", Cases: []reviewCase{}}
	for _, item := range dataset.Cases {
		review := reviewCase{inputCase: item, Candidates: []candidate{}, GoldChunkIDs: []uuid.UUID{}, ExpectedPoints: []string{}}
		q := queryTerms(item.Question)
		var rows interface {
			Next() bool
			Scan(...any) error
			Err() error
			Close()
		}
		if q != "" {
			rows, err = pool.Query(ctx, `WITH ranked AS (SELECT c.id,s.title,s.author,p.scan_page_index+1 AS scan_position,coalesce(p.printed_label,'') AS printed_page,p.image_url,c.text_exact,row_number() OVER (PARTITION BY s.id ORDER BY ts_rank_cd(c.search_vector,to_tsquery('english',$2)) DESC,c.id) AS source_rank FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE s.title=ANY($1::text[]) AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND p.page_kind='text' AND c.search_vector @@ to_tsquery('english',$2)) SELECT id,title,author,scan_position,printed_page,image_url,text_exact FROM ranked WHERE source_rank<=CASE WHEN cardinality($1::text[])>1 THEN 5 ELSE 10 END ORDER BY title,source_rank LIMIT 10`, item.SourceTitles, q)
		}
		if err != nil {
			log.Fatal(err)
		}
		if rows != nil {
			for rows.Next() {
				var c candidate
				if err = rows.Scan(&c.ChunkID, &c.Title, &c.Author, &c.ScanPosition, &c.PrintedPage, &c.ImageURL, &c.Passage); err != nil {
					log.Fatal(err)
				}
				review.Candidates = append(review.Candidates, c)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				log.Fatal(err)
			}
		}
		if len(review.Candidates) == 0 {
			fallback, queryErr := pool.Query(ctx, `SELECT c.id,s.title,s.author,p.scan_page_index+1,coalesce(p.printed_label,''),p.image_url,c.text_exact FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE s.title=ANY($1::text[]) AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND p.page_kind='text' ORDER BY p.scan_page_index,c.chunk_index LIMIT 10`, item.SourceTitles)
			if queryErr != nil {
				log.Fatal(queryErr)
			}
			for fallback.Next() {
				var c candidate
				if err = fallback.Scan(&c.ChunkID, &c.Title, &c.Author, &c.ScanPosition, &c.PrintedPage, &c.ImageURL, &c.Passage); err != nil {
					log.Fatal(err)
				}
				review.Candidates = append(review.Candidates, c)
			}
			err = fallback.Err()
			fallback.Close()
			if err != nil {
				log.Fatal(err)
			}
		}
		packet.Cases = append(packet.Cases, review)
		fmt.Printf("%s: %d candidate passages\n", item.ID, len(review.Candidates))
	}
	encoded, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(*output, append(encoded, '\n'), 0644); err != nil {
		log.Fatal(err)
	}
	var guide strings.Builder
	guide.WriteString("# Source-page review worksheet\n\nCandidate passages are search suggestions, not approved gold evidence. Open the original scan for each selected passage, then record the exact chunk IDs, expected answer points, reviewer, and date in `questions.draft.json`. Unsupported cases require a deliberate corpus-limit decision. Held-out cases must not be used while tuning.\n\n")
	for _, item := range packet.Cases {
		fmt.Fprintf(&guide, "## %s — %s\n\n%s\n\nSources: %s. Held out: %t.\n\nGold chunk IDs: _reviewer to select_. Expected points: _reviewer to write_.\n\n", item.ID, item.Category, item.Question, strings.Join(item.SourceTitles, "; "), item.HeldOut)
		for i, c := range item.Candidates {
			passage := strings.Join(strings.Fields(c.Passage), " ")
			if len([]rune(passage)) > 650 {
				passage = string([]rune(passage)[:650]) + "…"
			}
			fmt.Fprintf(&guide, "%d. **%s**, %s, scan %d%s — [original scan](%s) — chunk `%s`\n\n   > %s\n\n", i+1, c.Title, c.Author, c.ScanPosition, func() string {
				if c.PrintedPage != "" {
					return ", printed page " + c.PrintedPage
				}
				return ""
			}(), c.ImageURL, c.ChunkID, passage)
		}
	}
	if err = os.WriteFile(*markdown, []byte(guide.String()), 0644); err != nil {
		log.Fatal(err)
	}
}
