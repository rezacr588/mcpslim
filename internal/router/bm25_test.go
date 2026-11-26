package router

import "testing"

func TestSearchRanksBestMatch(t *testing.T) {
	ix := New()
	ix.Add("read_file", "read_file Read the contents of a file from disk path")
	ix.Add("search_web", "search_web Search the public web for a query")
	ix.Add("send_email", "send_email Send an email message to a recipient")

	hits := ix.Search("read a file", 2)
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].ID != "read_file" {
		t.Fatalf("top hit = %q, want read_file", hits[0].ID)
	}
}

func TestSearchNoMatch(t *testing.T) {
	ix := New()
	ix.Add("read_file", "read_file Read the contents of a file from disk")
	if hits := ix.Search("kubernetes cluster", 3); len(hits) != 0 {
		t.Fatalf("expected no hits, got %v", hits)
	}
}

func TestAddReplacesDocument(t *testing.T) {
	ix := New()
	ix.Add("t", "alpha beta")
	ix.Add("t", "gamma delta")
	if ix.Len() != 1 {
		t.Fatalf("len = %d, want 1", ix.Len())
	}
	if hits := ix.Search("alpha", 1); len(hits) != 0 {
		t.Fatalf("stale terms still indexed: %v", hits)
	}
	if hits := ix.Search("gamma", 1); len(hits) != 1 {
		t.Fatalf("new terms missing: %v", hits)
	}
}

func TestSearchDeterministicTieBreak(t *testing.T) {
	ix := New()
	ix.Add("b", "same text")
	ix.Add("a", "same text")
	hits := ix.Search("same", 2)
	if len(hits) != 2 || hits[0].ID != "a" || hits[1].ID != "b" {
		t.Fatalf("hits = %v, want a then b", hits)
	}
}
