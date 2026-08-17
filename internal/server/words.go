package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

	// English is the default language
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "English"
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	/* Get graph pathways */
	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, `
		MATCH path = (n: Word {term: $word, lang: $lang})-[r:CHILD_OF*]->(m: Word)
		WHERE n.reltype <> "cognate_of" AND all(innerNode IN nodes(path) WHERE innerNode.reltype IS NULL OR innerNode.reltype <> "cognate_of")
		RETURN path
	`,
		map[string]any{
			"word": unescapeParam(r, "word"),
			"lang": lang,
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	records := make([]map[string]any, len(result.Records))
	familySet := map[string]struct{}{}
	family := make([]string, 0, len(familySet))

	for i, record := range result.Records {
		records[i] = record.AsMap()

		path := record.AsMap()["path"].(neo4j.Path)

		for _, node := range path.Nodes {
			lang := node.Props["lang"].(string)

			if lang != "English" && lang != "Middle English" {
				familySet[lang] = struct{}{}
				family = append(family, lang)
			}
		}
	}

	family = append(family, "English")

	// Get families
	result, err = neo4j.ExecuteQuery(s.ctx, s.driver, `
		UNWIND $langs AS langName
		MATCH (l:Language)
		WHERE l.name CONTAINS langName
		WITH collect(DISTINCT l.glottocode) AS codes


		UNWIND codes AS code
		MATCH (n:Family)
		WHERE n.name CONTAINS '[' + code + ']'
		WITH collect(DISTINCT n) AS targets
		
		UNWIND range(0, size(targets) - 2) AS i
		UNWIND range(i + 1, size(targets) - 1) AS j
		WITH targets[i] AS a, targets[j] AS b
		
		MATCH path1 = (lca:Family)-[:PARENT_OF*0..]->(a)
		MATCH path2 = (lca)-[:PARENT_OF*0..]->(b)
		WHERE coalesce(lca.ignore, false) = false
		WITH a, b, lca, length(path1) + length(path2) AS totalDistance
		ORDER BY a.name, b.name, totalDistance ASC
		WITH a, b, collect(lca)[0] AS lca
		
		MATCH pathA = (lca)-[:PARENT_OF*0..]->(a)
		WITH a, b, lca, [x IN nodes(pathA) WHERE x <> lca] AS descA
		WITH a, b, lca, [x IN descA WHERE coalesce(x.ignore, false) = false][0] AS foundBranchA

		MATCH pathB = (lca)-[:PARENT_OF*0..]->(b)
		WITH a, b, lca, foundBranchA, [x IN nodes(pathB) WHERE x <> lca] AS descB
		WITH a, b, lca, foundBranchA, [x IN descB WHERE coalesce(x.ignore, false) = false][0] AS foundBranchB
		
		WITH coalesce(foundBranchA, lca) AS branchA, coalesce(foundBranchB, lca) AS branchB
		
		WITH collect(branchA) + collect(branchB) AS allBranches
		UNWIND allBranches AS branch
		WITH DISTINCT branch
		WHERE branch IS NOT NULL
		RETURN collect(branch.name) AS branches
	`,
		map[string]any{
			"langs": family,
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	rawList, _ := result.Records[0].Get("branches")
	branches := make([]string, 0, len(rawList.([]interface{})))
	for _, v := range rawList.([]interface{}) {
		branches = append(branches, v.(string))
	}
	fmt.Println(branches)

	/* Get flat geojson polygons */
	var params map[string]any
	cypher := `MATCH (l:Language) WHERE ANY(prefix IN $prefixes WHERE l.name CONTAINS prefix) RETURN l.name AS name, l.geometryJSON AS json`
	params = map[string]any{"prefixes": family}

	result, err = neo4j.ExecuteQuery(
		s.ctx,
		s.driver,
		cypher,
		params,
		neo4j.EagerResultTransformer,           // Safely packs records into memory
		neo4j.ExecuteQueryWithReadersRouting(), // Routes optimization for read-only query
	)
	if err != nil {
		fmt.Printf("failed to execute query: %v", err)
		return
	}

	features := make([]any, 0, len(result.Records))
	for _, record := range result.Records {
		name, _ := record.Get("name")
		nameStr, _ := name.(string)

		geometryJSON, _ := record.Get("json")
		geometryStr, _ := geometryJSON.(string)

		var geometry any
		if err := json.Unmarshal([]byte(geometryStr), &geometry); err != nil {
			continue
		}

		features = append(features, map[string]any{
			"type": "Feature",
			"properties": map[string]any{
				"name": nameStr,
			},
			"geometry": geometry,
		})
	}

	response := map[string]any{
		"graph":  records,
		"family": branches,
		"geojson": map[string]any{
			"type":     "FeatureCollection",
			"features": features,
		},
	}

	// Write to cache so that future queries are quick
	encoded, _ := json.Marshal(response)
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "English" {
		s.logger.Warn("history not implemented for non-english")
		json.NewEncoder(w).Encode(map[string]string{"error": "history not implemented for non-english"})
		return
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	word := chi.URLParam(r, "word")
	if decoded, err := neturl.PathUnescape(word); err == nil {
		word = decoded
	}

	req, err := http.NewRequest("GET", "https://www.etymonline.com/search?q="+neturl.QueryEscape(word), nil)
	if err != nil {
		log.Printf("Failed to build request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
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
	adNoise := regexp.MustCompile(`(?i)(ABCDEFGHIJKLMNOPQRSTUVWXYZ|Advertisement|Remove Ads|Want to remove ads\?[^.]*\.|allnamephraserootword parts|\d+ entries found\.|Related entries & more|Trending[^.]*\.)`)

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
		text = adNoise.ReplaceAllString(text, "")
		text = strings.TrimSpace(text)
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
		raw = adNoise.ReplaceAllString(raw, "")
		raw = strings.TrimSpace(raw)
		entries = append(entries, Entry{Word: word, Text: raw})
	}

	// Build a single text blob from entries matching the search word
	var raw strings.Builder
	for _, e := range entries {
		if !entryMatchesWord(e.Word, word) {
			continue
		}
		if e.Word != "" {
			raw.WriteString(e.Word + ": ")
		}
		raw.WriteString(e.Text + "\n\n")
	}
	if raw.Len() == 0 && len(entries) > 0 {
		e := entries[0]
		raw.WriteString(e.Word + ": ")
		raw.WriteString(e.Text + "\n\n")
	}

	history, err := s.ai.Prompt(r.Context(), fmt.Sprintf(
		"%s\n\nWrite one sentence about the origin and history of %q.",
		raw.String(), word,
	))
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

func entryMatchesWord(entryWord, searchWord string) bool {
	clean := strings.TrimSpace(strings.SplitN(entryWord, "(", 2)[0])
	clean = strings.TrimRight(clean, " ")
	return strings.EqualFold(clean, searchWord)
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

func unescapeParam(r *http.Request, param string) string {
	word := chi.URLParam(r, param)
	if decoded, err := neturl.PathUnescape(word); err == nil {
		return decoded
	}
	return word
}

func (s *Server) handleGetIpa(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "English" {
		s.logger.Warn("ipa not implemented for non-english")
		json.NewEncoder(w).Encode(map[string]string{"error": "ipa not implemented for non-english"})
		return
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}
	s.logger.Error("cache lookup failed", "error", err)

	word := chi.URLParam(r, "word")
	if decoded, err := neturl.PathUnescape(word); err == nil {
		word = decoded
	}

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
