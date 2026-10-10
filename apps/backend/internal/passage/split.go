// Package passage prepares exact, location-preserving reading spans. Heading
// recognition is a layout hint, never a verified remedy identity or assertion.
package passage

import (
	"strings"
	"unicode"
)

const Version = "section-safe-v1"

type Span struct {
	Start, End int
	Text       string
}

// SectionLabel recognizes conventional profile labels only at the start of a
// line, either alone or followed by punctuation. It never matches running prose.
func SectionLabel(line string) string {
	line = strings.TrimSpace(line)
	for _, label := range []string{"mental symptoms", "respiratory organs", "urinary organs", "female organs", "male organs", "extremities", "generalities", "relationship", "relationships", "modalities", "aggravation", "amelioration", "abdomen", "stomach", "vertigo", "throat", "rectum", "urinary", "female", "male", "chest", "heart", "sleep", "fever", "mouth", "mind", "head", "eyes", "ears", "nose", "face", "back", "skin", "dose"} {
		if len(line) < len(label) || !strings.EqualFold(line[:len(label)], label) {
			continue
		}
		rest := strings.TrimSpace(line[len(label):])
		if rest == "" || strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "—") || strings.HasPrefix(rest, "--") {
			return label
		}
	}
	return ""
}

func Heading(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if SectionLabel(line) != "" {
		return true
	}
	if len([]rune(line)) > 80 || len(strings.Fields(line)) > 6 {
		return false
	}
	letters := 0
	for _, r := range line {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) {
				return false
			}
		} else if !unicode.IsSpace(r) && r != '-' && r != '(' && r != ')' {
			return false
		}
	}
	return letters >= 3
}

// Split never crosses a recognized uppercase entry heading or the caller's page/block boundary.
// Offsets are Unicode code points into text, without normalization or synthesis.
func Split(text string, limit int) []Span {
	if limit < 1 {
		return nil
	}
	r := []rune(text)
	boundaries := []int{0}
	for i := 0; i < len(r); {
		end := i
		for end < len(r) && r[end] != '\n' {
			end++
		}
		if i > 0 && Heading(string(r[i:end])) && SectionLabel(string(r[i:end])) == "" {
			boundaries = append(boundaries, i)
		}
		i = end + 1
	}
	boundaries = append(boundaries, len(r))
	out := []Span{}
	for b := 0; b+1 < len(boundaries); b++ {
		stop := boundaries[b+1]
		for start := boundaries[b]; start < stop; {
			for start < stop && unicode.IsSpace(r[start]) {
				start++
			}
			if start == stop {
				break
			}
			end := start + limit
			if end > stop {
				end = stop
			} else if end < stop {
				// Prefer a paragraph or sentence ending, then a word boundary. A plain
				// line break may be OCR wrapping, so it is not automatically a sentence.
				chosen := 0
				for j := end; j > start+limit/2; j-- {
					if r[j-1] == '\n' && j >= 2 && r[j-2] == '\n' {
						chosen = j
						break
					}
					if unicode.IsSpace(r[j]) && strings.ContainsRune(".!?;", r[j-1]) {
						chosen = j
						break
					}
				}
				if chosen == 0 {
					for j := end; j > start+limit/2; j-- {
						if unicode.IsSpace(r[j]) {
							chosen = j
							break
						}
					}
				}
				if chosen > 0 {
					end = chosen
				}
			}
			next := end
			for end > start && unicode.IsSpace(r[end-1]) {
				end--
			}
			if end > start {
				out = append(out, Span{start, end, string(r[start:end])})
			}
			start = next
		}
	}
	return out
}
