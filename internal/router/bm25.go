// Package router implements a small BM25 index over MCP tool descriptions.
// It has no dependencies and is safe for concurrent search after Build.
package router

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	k1 = 1.2
	b  = 0.75
)

type document struct {
	id     string
	tokens []string
	length int
}

// Index is a BM25 inverted index.
type Index struct {
	docs   []document
	df     map[string]int
	avgLen float64
	byID   map[string]int
	built  bool
}

// New returns an empty index.
func New() *Index {
	return &Index{df: make(map[string]int), byID: make(map[string]int)}
}

// Add indexes text under id. Adding the same id twice keeps the last text.
func (ix *Index) Add(id, text string) {
	tokens := Tokenize(text)
	if i, ok := ix.byID[id]; ok {
		ix.remove(i)
	}
	ix.byID[id] = len(ix.docs)
	ix.docs = append(ix.docs, document{id: id, tokens: tokens, length: len(tokens)})
	ix.built = false
}

// Build finalises document frequencies. Search calls it lazily.
func (ix *Index) Build() {
	if ix.built {
		return
	}
	ix.df = make(map[string]int, len(ix.df))
	total := 0
	for _, d := range ix.docs {
		seen := make(map[string]struct{}, len(d.tokens))
		for _, t := range d.tokens {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			ix.df[t]++
		}
		total += d.length
	}
	if len(ix.docs) > 0 {
		ix.avgLen = float64(total) / float64(len(ix.docs))
	}
	ix.built = true
}

// Hit is one search result.
type Hit struct {
	ID    string
	Score float64
}

// Search returns up to k documents ranked by BM25 score. Ties break by id so
// results are deterministic.
func (ix *Index) Search(query string, k int) []Hit {
	if k <= 0 {
		k = 5
	}
	ix.Build()
	if len(ix.docs) == 0 {
		return nil
	}
	terms := unique(Tokenize(query))
	if len(terms) == 0 {
		return nil
	}
	n := float64(len(ix.docs))
	scores := make(map[int]float64, len(ix.docs))
	for _, term := range terms {
		df := float64(ix.df[term])
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for i, d := range ix.docs {
			tf := float64(count(d.tokens, term))
			if tf == 0 {
				continue
			}
			denom := tf + k1*(1-b+b*float64(d.length)/ix.avgLen)
			scores[i] += idf * tf * (k1 + 1) / denom
		}
	}
	hits := make([]Hit, 0, len(scores))
	for i, score := range scores {
		hits = append(hits, Hit{ID: ix.docs[i].id, Score: score})
	}
	sort.Slice(hits, func(a, c int) bool {
		if hits[a].Score == hits[c].Score {
			return hits[a].ID < hits[c].ID
		}
		return hits[a].Score > hits[c].Score
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// Len reports the number of indexed documents.
func (ix *Index) Len() int { return len(ix.docs) }

func (ix *Index) remove(i int) {
	id := ix.docs[i].id
	delete(ix.byID, id)
	ix.docs = append(ix.docs[:i], ix.docs[i+1:]...)
	for name, idx := range ix.byID {
		if idx > i {
			ix.byID[name] = idx - 1
		}
	}
	ix.built = false
}

// Tokenize lowercases text and splits it on anything that is not a letter or
// digit. Tokens shorter than two characters are dropped.
func Tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func unique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if len(s) < 2 {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func count(tokens []string, term string) int {
	n := 0
	for _, t := range tokens {
		if t == term {
			n++
		}
	}
	return n
}
