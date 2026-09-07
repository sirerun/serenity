package index

import "testing"

func TestSearchFTSNaturalLanguage(t *testing.T) {
	eng, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	if err := eng.InsertChunk(t.Context(), "literal", "", `conformance serenity-pinned-v1 protocol round-trip he said "hello"`, "", "note"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"conformance serenity-pinned-v1 protocol round-trip", `"hello"`, "round-trip"} {
		hits, err := eng.SearchFTS(t.Context(), LiteralFTSQuery(query), 10)
		if err != nil || len(hits) != 1 {
			t.Errorf("query %q: hits=%v err=%v", query, hits, err)
		}
	}
	for _, query := range []string{" ", "(", "***", `"`, "title:absent"} {
		hits, err := eng.SearchFTS(t.Context(), LiteralFTSQuery(query), 10)
		if err != nil || len(hits) != 0 {
			t.Errorf("literal punctuation query %q: hits=%v err=%v", query, hits, err)
		}
	}
}
