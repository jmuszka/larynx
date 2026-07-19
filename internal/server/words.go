package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/go-chi/chi/v5"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func (s *Server) wordsRouter() http.Handler {
	r := chi.NewRouter()

	r.Get("/{word}/etymology", s.handleGetEtymology)
	r.Get("/{word}/history", s.handleGetHistory)
	// r.Get("/{word}/definition", s.handleGetDefinition)
	r.Get("/{word}/ipa", s.handleGetIpa)
	r.Get("/", s.handleSearchWords)

	return r
}

func (s *Server) handleGetEtymology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	/* Get graph pathways */
	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, `
		MATCH path = (n: Word {term: $word, lang: "English"})-[r:CHILD_OF*]->(m: Word)
		WHERE n.reltype <> "cognate_of" AND all(innerNode IN nodes(path) WHERE innerNode.reltype IS NULL OR innerNode.reltype <> "cognate_of")
		RETURN path
	`,
		map[string]any{
			"word": chi.URLParam(r, "word"),
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	records := make([]map[string]any, len(result.Records))
	var family []string

	for i, record := range result.Records {
		records[i] = record.AsMap()

		path := record.AsMap()["path"].(neo4j.Path)

		for _, node := range path.Nodes {
			lang := node.Props["lang"].(string)

			if lang != "English" && lang != "Middle English" {
				family = append(family, lang)
			}
		}
	}

	/* Get flat geojson polygons */
	var clauses []string
	params := make(map[string]interface{})

	for i, prefix := range family {
		paramName := fmt.Sprintf("prefix%d", i)
		clauses = append(clauses, fmt.Sprintf("l.name STARTS WITH $%s", paramName))
		params[paramName] = prefix
	}

	whereClause := strings.Join(clauses, " OR ")
	cypher := fmt.Sprintf(`
			MATCH (l:Language)
			WHERE %s
			RETURN l.name AS name, l.json AS json
		`, whereClause)

	result, err = neo4j.ExecuteQuery(
		s.ctx,
		s.driver,
		cypher,
		params,
		neo4j.EagerResultTransformer,           // Safely packs records into memory
		neo4j.ExecuteQueryWithReadersRouting(), // Routes optimization for read-only query
	)
	if err != nil {
		fmt.Printf("failed to execute query: %w", err)
		return
	}

	var results []string
	for _, record := range result.Records {
		geojson, _ := record.Get("json")
		geojsonStr, _ := geojson.(string)

		results = append(results, geojsonStr)
	}

	response := map[string]any{
		"graph":   records,
		"family":  family,
		"geojson": results,
	}

	// Write to cache so that future queries are quick
	encoded, _ := json.Marshal(response)
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	word := chi.URLParam(r, "word")

	req, err := http.NewRequest("GET", "https://www.etymonline.com/search?q="+neturl.QueryEscape(word), nil)
	if err != nil {
		log.Printf("Failed to build request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to fetch etymology page: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("Failed to parse HTML: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	doc.Find("script, style, nav, header, footer").Remove()

	type Entry struct {
		Word string `json:"word"`
		Text string `json:"text"`
	}

	var entries []Entry
	whitespace := regexp.MustCompile(`\s{2,}`)

	// etymonline uses CSS modules — class names follow the pattern "word--*"
	doc.Find("[class*='word--']").Each(func(_ int, sel *goquery.Selection) {
		name := strings.TrimSpace(sel.Find("[class*='word__name']").Text())
		if name == "" {
			name = strings.TrimSpace(sel.Find("h1, h2, h3").First().Text())
		}
		text := strings.TrimSpace(sel.Find("[class*='word__defination'], [class*='word__def'], section").Text())
		if text == "" {
			text = strings.TrimSpace(sel.Find("p").Text())
		}
		text = whitespace.ReplaceAllString(text, " ")
		if name != "" && text != "" {
			entries = append(entries, Entry{Word: name, Text: text})
		}
	})

	// Fall back to main content text if no structured entries matched
	if len(entries) == 0 {
		raw := doc.Find("main, [role='main'], article").Text()
		if raw == "" {
			raw = doc.Find("body").Text()
		}
		raw = whitespace.ReplaceAllString(strings.TrimSpace(raw), "\n")
		entries = append(entries, Entry{Word: word, Text: raw})
	}

	// Build a single text blob from all scraped entries to pass to the LLM
	var raw strings.Builder
	for _, e := range entries {
		if e.Word != "" {
			raw.WriteString(e.Word + ": ")
		}
		raw.WriteString(e.Text + "\n\n")
	}

	history, err := formatWordHistory(r.Context(), word, raw.String())
	if err != nil {
		log.Printf("LLM formatting failed: %v", err)
		json.NewEncoder(w).Encode(map[string]any{"word": word, "results": entries})
		return
	}

	response := map[string]any{
		"word":    word,
		"history": history,
	}

	// Write to cache so that future queries are quick
	encoded, _ := json.Marshal(response)
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}

func formatWordHistory(ctx context.Context, word, rawText string) (string, error) {
	client := anthropic.NewClient()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       anthropic.ModelClaudeHaiku4_5,
		MaxTokens:   256,
		Temperature: anthropic.Float(0.0),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf(
				"Here is raw etymological data for the word %q scraped from etymonline.com:\n\n%s\n\nSummarize this word's etymology and historical development in one or two complete sentences. Respond with plain prose only: no markdown, headers, titles, preamble, or bullet points. Most importantly, BE CONCISE.",
				word, rawText,
			))),
		},
	})
	if err != nil {
		return "", err
	}

	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return tb.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in LLM response")
}

// TODO: implement
func (s *Server) handleGetDefinition(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleGetIpa(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	word := chi.URLParam(r, "word")

	var ipa string

	// Retrieve blogpost
	err = s.db.QueryRow(
		"SELECT ipa FROM ipa WHERE word LIKE ?",
		word,
	).Scan(&ipa)
	if err != nil {
		log.Printf("Query failed: %v", err)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	response := map[string]string{
		"ipa": ipa,
	}

	// Write to cache so that future queries are quick
	encoded, _ := json.Marshal(response)
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}
