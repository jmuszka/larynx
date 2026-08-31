package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func (s *Server) wordsRouter() http.Handler {
	r := chi.NewRouter()

	r.With(httprate.LimitBy(rateLimitEtymologyPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/etymology", s.handleGetEtymology)
	r.With(httprate.LimitBy(rateLimitHistoryPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/history", s.handleGetHistory)
	// r.Get("/{word}/definition", s.handleGetDefinition)
	r.Get("/{word}/ipa", s.handleGetIpa)
	r.Get("/", s.handleSearchWords)
	return r
}

type etymologyResponse struct {
	Graph   []map[string]any `json:"graph"`
	Family  []string         `json:"family"`
	GeoJSON geoJSON          `json:"geojson"`
}

type geoJSON struct {
	Type     string    `json:"type" example:"FeatureCollection"`
	Features []feature `json:"features"`
}

type feature struct {
	Type       string         `json:"type" example:"Feature"`
	Properties map[string]any `json:"properties"`
	Geometry   map[string]any `json:"geometry"`
}

// Note: in the Wikitionary dataset, the longest word was empirically measured at 204 characters and longest language at 58 characters
const (
	maxWordLength = 250
	maxLangLength = 100
)

// handleGetEtymology godoc
// @Summary      Get a word's etymology graph
// @Description  Returns the graph of ancestor words, their language families, and a GeoJSON map of where those languages are spoken.
// @Tags         words
// @Produce      json
// @Param        word  path      string  true  "The word to look up"
// @Param        lang  query     string  false "Language of the word"  default(English)
// @Success      200   {object}  etymologyResponse
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Router       /words/{word}/etymology [get]
func (s *Server) handleGetEtymology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Input validation
	lang := r.URL.Query().Get("lang")
	if len(lang) > maxLangLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("lang exceeds maximum length of %d characters", maxLangLength))
		return
	}

	// English is the default language
	if lang == "" {
		lang = "English"
	}

	// Input validation
	word := unescapeParam(r, "word")
	if len(word) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "word is required")
		return
	}
	if len(word) > maxWordLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("word exceeds maximum length of %d characters", maxWordLength))
		return
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	/* Get graph pathways */
	cypher := `
		MATCH path = (n: Word {term: $word, lang: $lang})-[r:CHILD_OF*0..]->(m: Word)
		WHERE n.reltype IS NULL OR (n.reltype<> 'cognate_of' AND all(innerNode IN nodes(path) WHERE innerNode.reltype IS NULL OR innerNode.reltype <> 'cognate_of'))
		RETURN path
	`
	params := map[string]any{
		"word": word,
		"lang": lang,
	}
	s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
	result, err := neo4j.ExecuteQuery(r.Context(), s.driver, cypher,
		params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.logger.Error("failed to execute etymology query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
		return
	}

	records := make([]map[string]any, len(result.Records))
	familySet := map[string]struct{}{}
	family := make([]string, 0, len(familySet))

	for i, record := range result.Records {
		records[i] = record.AsMap()

		path, ok := record.AsMap()["path"].(neo4j.Path)
		if !ok {
			continue
		}

		for _, node := range path.Nodes {
			lang, ok := node.Props["lang"].(string)
			if !ok {
				continue
			}

			if lang != "English" && lang != "Middle English" {
				familySet[lang] = struct{}{}
				family = append(family, lang)
			}
		}
	}

	family = append(family, "English")

	// Get families
	cypher = `
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
	`
	params = map[string]any{
		"langs": family,
	}
	s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
	result, err = neo4j.ExecuteQuery(r.Context(), s.driver, cypher,
		params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.logger.Error("failed to execute families query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
		return
	}

	branches := []string{}
	if len(result.Records) > 0 {
		if rawList, ok := result.Records[0].Get("branches"); ok {
			if list, ok := rawList.([]interface{}); ok {
				for _, v := range list {
					if s, ok := v.(string); ok {
						branches = append(branches, s)
					}
				}
			}
		}
	}

	/* Get flat geojson polygons */
	cypher = `MATCH (l:Language) WHERE ANY(prefix IN $prefixes WHERE l.name CONTAINS prefix) RETURN l.name AS name, l.geometryJSON AS json`
	params = map[string]any{"prefixes": family}

	s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
	result, err = neo4j.ExecuteQuery(
		r.Context(),
		s.driver,
		cypher,
		params,
		neo4j.EagerResultTransformer,           // Safely packs records into memory
		neo4j.ExecuteQueryWithReadersRouting(), // Routes optimization for read-only query
	)
	if err != nil {
		s.logger.Error("failed to execute geojson query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
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
	encoded, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}

type historyResponse struct {
	Word    string `json:"word"`
	History string `json:"history"`
}

// handleGetHistory godoc
// @Summary      Get a word's origin and history
// @Description  Returns a one-sentence summary of a word's origin and history.
// @Tags         words
// @Produce      json
// @Param        word  path      string  true  "The word to look up"
// @Param        lang  query     string  false "Language of the word"  default(English)
// @Success      200   {object}  historyResponse
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Router       /words/{word}/history [get]
func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	lang := r.URL.Query().Get("lang")
	if len(lang) > maxLangLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("lang exceeds maximum length of %d characters", maxLangLength))
		return
	}
	if lang != "" && lang != "English" {
		s.logger.Warn("history not implemented for non-english")
		s.writeJSONError(w, http.StatusBadRequest, "history not implemented for non-english")
		return
	}

	// Input validation
	word := unescapeParam(r, "word")
	if len(word) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "word is required")
		return
	}
	if len(word) > maxWordLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("word exceeds maximum length of %d characters", maxWordLength))
		return
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://www.etymonline.com/search?q="+neturl.QueryEscape(word), nil)
	if err != nil {
		s.logger.Error("failed to build request", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("failed to fetch etymology page", "error", err)
		s.writeJSONError(w, http.StatusBadGateway, "failed to fetch etymology source")
		return
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		s.logger.Error("failed to parse HTML", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to parse etymology source")
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
		s.logger.Error("LLM formatting failed", "error", err)
		s.writeJSON(w, http.StatusOK, map[string]any{"word": word, "results": entries})
		return
	}

	response := map[string]any{
		"word":    word,
		"history": history,
	}

	// Write to cache so that future queries are quick
	encoded, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
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
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "Not implemented",
	})
}

// handleSearchWords godoc
// @Summary      Search for words
// @Description  Returns English words whose term starts with the given prefix.
// @Tags         words
// @Produce      json
// @Param        prefix  query     string  true  "Prefix to search for"
// @Success      200     {array}   string
// @Failure      500     {object}  map[string]string
// @Security     BearerAuth
// @Router       /words [get]
func (s *Server) handleSearchWords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	// Parse GET parameters
	prefix := r.URL.Query().Get("prefix")

	// Input validation
	if len(prefix) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "prefix is required")
		return
	}
	if len(prefix) > maxWordLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("prefix exceeds maximum length of %d characters", maxWordLength))
		return
	}

	// Construct Cypher query
	const query = `
		MATCH (n:Word { lang: 'English' })
		WHERE n.term IS NOT NULL AND n.term STARTS WITH toLower($prefix)
		RETURN DISTINCT n.term AS term
		ORDER BY size(term), term ASC
	`

	// Fetch search results from Neo4j
	searchParams := map[string]any{"prefix": prefix}
	s.logger.Debug("CYPHER: " + renderCypher(query, searchParams))
	result, err := neo4j.ExecuteQuery(r.Context(), s.driver, query,
		searchParams, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.logger.Error("failed to execute search query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
		return
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

	// Write to cache so that future queries are quick
	encoded, err := json.Marshal(terms)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}

func unescapeParam(r *http.Request, param string) string {
	word := chi.URLParam(r, param)
	if decoded, err := neturl.PathUnescape(word); err == nil {
		return decoded
	}
	return word
}

type ipaResponse struct {
	IPA string `json:"ipa"`
}

// handleGetIpa godoc
// @Summary      Get a word's IPA transcription
// @Description  Returns the International Phonetic Alphabet transcription for a word.
// @Tags         words
// @Produce      json
// @Param        word  path      string  true  "The word to look up"
// @Param        lang  query     string  false "Language of the word"  default(English)
// @Success      200   {object}  ipaResponse
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Router       /words/{word}/ipa [get]
func (s *Server) handleGetIpa(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	lang := r.URL.Query().Get("lang")
	if len(lang) > maxLangLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("lang exceeds maximum length of %d characters", maxLangLength))
		return
	}
	if lang != "" && lang != "English" {
		s.logger.Warn("ipa not implemented for non-english")
		s.writeJSONError(w, http.StatusBadRequest, "ipa not implemented for non-english")
		return
	}

	// Input validation
	word := unescapeParam(r, "word")
	if len(word) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "word is required")
		return
	}
	if len(word) > maxWordLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("word exceeds maximum length of %d characters", maxWordLength))
		return
	}

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}
	s.logger.Error("cache lookup failed", "error", err)

	var ipa string

	// Retrieve blogpost
	const sqlQuery = "SELECT ipa FROM ipa WHERE word LIKE ?"
	s.logger.Debug("SQL: " + renderSQL(sqlQuery, []any{word}))
	err = s.db.QueryRow(
		sqlQuery,
		word,
	).Scan(&ipa)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.writeJSONError(w, http.StatusNotFound, "ipa not found")
			return
		}
		s.logger.Error("query failed", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to query database")
		return
	}

	response := map[string]string{
		"ipa": ipa,
	}

	// Write to cache so that future queries are quick
	encoded, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}
