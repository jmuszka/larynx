package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func (s *Server) wordsRouter() http.Handler {
	r := chi.NewRouter()

	r.Get("/{word}/etymology", s.handleGetEtymology)
	// r.Get("/{word}/history", s.handleGetHistory)
	// r.Get("/{word}/definition", s.handleGetDefinition)
	// r.Get("/{word}/ipa", s.handleGetIpa)
	r.Get("/", s.handleSearchWords)

	return r
}

func (s *Server) handleGetEtymology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, `
		MATCH (n:Word)
		WHERE n.lang = "English"
		AND n.term IS NOT NULL AND n.term =~ $word
		RETURN n
	`,
		map[string]any{
			"word": "(?i)" + chi.URLParam(r, "word"),
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	records := make([]map[string]any, len(result.Records))
	for i, record := range result.Records {
		records[i] = record.AsMap()
	}

	json.NewEncoder(w).Encode(records)
}

// TODO: implement
func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGetDefinition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGetIpa(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

func (s *Server) handleSearchWords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse GET parameters
	prefix := r.URL.Query().Get("prefix")

	// Construct Cypher query
	const query = `
		MATCH (n:Word { lang: "English" })
		WHERE n.term IS NOT NULL AND n.term STARTS WITH toLower($prefix)
		RETURN DISTINCT n.term AS term
		ORDER BY size(term), term ASC
	`

	// Fetch search results from Neo4j
	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, query,
		map[string]any{
			"prefix": prefix,
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	// Package search results into an array
	terms := make([]string, 0, len(result.Records))
	for _, record := range result.Records {
		if term, ok := record.Get("term"); ok {
			if s, ok := term.(string); ok {
				terms = append(terms, s)
			}
		}
	}

	json.NewEncoder(w).Encode(terms)
}
