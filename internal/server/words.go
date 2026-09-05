package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

var (
	htmlTagRegexp = regexp.MustCompile(`(?s)<[^>]*>`)
	adNoiseRegexp = regexp.MustCompile(`(?i)(ABCDEFGHIJKLMNOPQRSTUVWXYZ|Advertisement|Remove Ads|Want to remove ads\?[^.]*\.|allnamephraserootword parts|\d+ entries found\.|Related entries & more|Trending[^.]*\.)`)
)

func (s *Server) wordsRouter() http.Handler {
	r := chi.NewRouter()

	r.With(httprate.LimitBy(rateLimitEtymologyPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/etymology", s.handleGetEtymology)
	r.With(httprate.LimitBy(rateLimitHistoryPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/history", s.handleGetHistory)
	// r.Get("/{word}/definition", s.handleGetDefinition)
	r.Get("/", s.handleSearchWords)
	return r
}

type etymologyResponse struct {
	Graph      []map[string]any `json:"graph"`
	FamilyTree *familyNode      `json:"familyTree"`
	GeoJSON    geoJSON          `json:"geojson"`
}

type familyNode struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Value    int           `json:"value"`
	Children []*familyNode `json:"children,omitempty"`
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

// Heatmap diffusion weights. Tier 1 (the ancestor language's own region) is
// hottest; tier 2 (immediate-family descendants) and tier 3 (parent-family
// descendants) are progressively cooler. Counts sum across overlapping tiers.
const (
	tier1Weight = 3
	tier2Weight = 2
	tier3Weight = 1
)

// handleGetEtymology godoc
// @Summary      Get a word's etymology graph
// @Description  Returns the graph of ancestor words, their language families, and a GeoJSON map of where those languages are spoken.
// @Tags         words
// @Produce      json
// @Param        word  path      string  true  "The word to look up"
// @Param        lang  query     string  false "Language of the word"  default(English)
// @Param        geojson  query  string  false "Include geojson in the response"  default(true)
// @Param        family   query  string  false "Include the language family tree in the response"  default(true)
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

	// Geojson is included by default, but can be disabled to skip the
	// expensive geometry query and shrink the response payload.
	includeGeojson := true
	if v := r.URL.Query().Get("geojson"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			includeGeojson = b
		}
	}

	// The family tree is included by default, but can be disabled to skip the
	// family hierarchy query.
	includeFamily := true
	if v := r.URL.Query().Get("family"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			includeFamily = b
		}
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
		MATCH path = (head: Word {term: $word, lang: $lang}) (()-[:abbreviation_of|` + "`back-formation_from`" + `|blend_of|borrowed_from|calque_of|clipping_of|compound_of|derived_from|doublet_with|has_affix|has_confix|has_prefix|has_prefix_with_root|has_root|has_suffix|inherited_from|initialism_of|is_onomatopoeic|learned_borrowing_from|named_after|orthographic_borrowing_from|` + "`phono-semantic_matching_of`" + `|semantic_loan_of|` + "`semi_learned_borrowing_from`" + `|unadapted_borrowing_from]->()){0,} (tail: Word)
		WITH head, tail, path
		ORDER BY length(path) DESC 
		RETURN head, tail, head(collect(path)) AS path
	`
	params := map[string]any{
		"word": word,
		"lang": lang,
	}
	s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
	result, err := s.graph.ExecuteQuery(r.Context(), cypher,
		params, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.logger.Error("failed to execute etymology query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
		return
	}

	if len(result.Records) == 0 {
		s.writeJSONError(w, http.StatusNotFound, "word not found")
		return
	}

	records := make([]map[string]any, len(result.Records))
	familySet := map[string]struct{}{}

	var ipa any
	for i, record := range result.Records {
		records[i] = record.AsMap()

		path, ok := record.AsMap()["path"].(neo4j.Path)
		if !ok {
			continue
		}

		if i == 0 && len(path.Nodes) > 0 {
			ipa = path.Nodes[0].Props["ipa"]
		}

		for _, node := range path.Nodes {
			lang, ok := node.Props["lang"].(string)
			if !ok {
				continue
			}

			// Remove Modern and Middle English to avoid polluting etymological composition
			if lang != "English" && lang != "Middle English" {
				familySet[lang] = struct{}{}
			}
		}
	}

	// Convert hash set to array
	langNames := make([]string, 0, len(familySet))
	for k := range familySet {
		langNames = append(langNames, k)
	}

	var familyTree *familyNode

	if includeFamily {
		// Get the family hierarchy. Each matched language sits directly under its
		// immediate family (Family -[:PARENT_OF]-> Language), so collect those
		// immediate families as the targets. For each target, return its lineage
		// consisting of the target itself plus every branching point: a node whose
		// children lead to at least two distinct targets (i.e. the LCAs). Nodes
		// that only lead to a single target are pruned, so the lineage never walks
		// above the highest LCA. Language nodes never appear.
		cypher = `
			UNWIND $langs AS langName
			MATCH (f:Family)-[:PARENT_OF]->(l:Language)
			WHERE l.name STARTS WITH langName
			WITH collect(DISTINCT f) AS targets

			UNWIND targets AS target
			MATCH path = (root:Family)-[:PARENT_OF*0..]->(target)
			WHERE NOT (root)<-[:PARENT_OF]-()
			WITH target, nodes(path) AS ns, targets
			RETURN [n IN ns
				WHERE n = target OR size([(n)-[:PARENT_OF]->(c)
					WHERE size([(c)-[:PARENT_OF*0..]->(t) WHERE t IN targets | t]) > 0 | c]) >= 2
				| n.name] AS lineage
		`
		params = map[string]any{
			"langs": langNames,
		}
		s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
		result, err = s.graph.ExecuteQuery(r.Context(), cypher,
			params, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			s.logger.Error("failed to execute families query", "error", err)
			s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
			return
		}

		lineages := make([][]string, 0, len(result.Records))
		for _, record := range result.Records {
			raw, ok := record.Get("lineage")
			if !ok {
				continue
			}
			list, ok := raw.([]interface{})
			if !ok {
				continue
			}
			lineage := make([]string, 0, len(list))
			for _, v := range list {
				if name, ok := v.(string); ok {
					lineage = append(lineage, name)
				}
			}
			lineages = append(lineages, lineage)
		}

		familyTree = buildFamilyTree(lineages)
	}

	var geojson any

	if includeGeojson {
		// Heatmap diffusion. For each ancestor language, emit its own region
		// (tier 1), every descendant of its immediate family (tier 2), and every
		// descendant of that family's parent (tier 3). Tiers carry descending
		// weights, and counts are summed per glottocode so overlapping polygons
		// render hotter while the payload stays small.
		cypher = `
			CALL {
				UNWIND $langs AS langName
				MATCH (l:Language) WHERE l.name STARTS WITH langName
				RETURN l.glottocode AS id, l.name AS name, l.geometryJSON AS json, $w1 AS weight
				UNION ALL
				UNWIND $langs AS langName
				MATCH (l:Language) WHERE l.name STARTS WITH langName
				MATCH (f:Family)-[:PARENT_OF]->(l)
				MATCH (f)-[:PARENT_OF*1..]->(d:Language)
				WHERE d <> l
				RETURN d.glottocode AS id, d.name AS name, d.geometryJSON AS json, $w2 AS weight
				UNION ALL
				UNWIND $langs AS langName
				MATCH (l:Language) WHERE l.name STARTS WITH langName
				MATCH (f:Family)-[:PARENT_OF]->(l)
				MATCH (g:Family)-[:PARENT_OF]->(f)
				MATCH (g)-[:PARENT_OF*1..]->(d:Language)
				RETURN d.glottocode AS id, d.name AS name, d.geometryJSON AS json, $w3 AS weight
			}
			WITH id, name, json, sum(weight) AS count
			RETURN id, name, json, count
			ORDER BY count DESC
		`
		params = map[string]any{"langs": langNames, "w1": tier1Weight, "w2": tier2Weight, "w3": tier3Weight}

		s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
		result, err = s.graph.ExecuteQuery(
			r.Context(),
			cypher,
			params,
			neo4j.ExecuteQueryWithReadersRouting(), // Routes optimization for read-only query
		)
		if err != nil {
			s.logger.Error("failed to execute geojson query", "error", err)
			s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
			return
		}

		features := make([]any, 0, len(result.Records))
		for _, record := range result.Records {
			id, _ := record.Get("id")
			idStr, _ := id.(string)

			name, _ := record.Get("name")
			nameStr, _ := name.(string)

			count, _ := record.Get("count")
			countInt, _ := count.(int64)

			geometryJSON, _ := record.Get("json")
			geometryStr, _ := geometryJSON.(string)

			var geometry any
			if err := json.Unmarshal([]byte(geometryStr), &geometry); err != nil {
				continue
			}

			features = append(features, map[string]any{
				"type": "Feature",
				"properties": map[string]any{
					"id":    idStr,
					"name":  nameStr,
					"count": countInt,
				},
				"geometry": geometry,
			})
		}

		geojson = map[string]any{
			"type":     "FeatureCollection",
			"features": features,
		}
	}

	response := map[string]any{
		"graph":      records,
		"familyTree": familyTree,
		"geojson":    geojson,
		"ipa":        ipa,
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

	req, err := http.NewRequestWithContext(r.Context(), "GET", s.cfg.EtymologyBaseURL+"/word/"+neturl.PathEscape(word), nil)
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

	// Each word sense is rendered as an h2 with the "font-serif" class, and its
	// definition paragraphs live in the same enclosing section. Related-entry
	// links use a span (not an h2), so they are excluded automatically.
	doc.Find("h2[class*='font-serif']").Each(func(_ int, heading *goquery.Selection) {
		name := strings.TrimSpace(heading.Find("[lang='en']").First().Text())
		if name == "" {
			name = strings.TrimSpace(heading.Text())
		}
		section := heading.Closest("section")
		if section.Length() == 0 {
			section = heading.Parent()
		}
		var text strings.Builder
		section.Find("p").Each(func(_ int, p *goquery.Selection) {
			if t := cleanEtymologyText(p.Text()); t != "" {
				text.WriteString(t + " ")
			}
		})
		if name != "" && text.Len() > 0 {
			entries = append(entries, Entry{Word: name, Text: strings.TrimSpace(text.String())})
		}
	})

	if len(entries) == 0 {
		s.writeJSONError(w, http.StatusNotFound, "no etymology entry found")
		return
	}

	// Build a single text blob from the matched word senses.
	var raw strings.Builder
	for _, e := range entries {
		raw.WriteString(e.Word + ": ")
		raw.WriteString(e.Text + "\n\n")
	}

	history, err := s.ai.Prompt(r.Context(),
		"You are a concise etymology assistant. Summarize the source text in 1-2 short, direct sentences covering the word's origin and key historical stages. Preserve all essential facts; add no filler or commentary.",
		fmt.Sprintf("%s\n\nSummarize the origin and history of %q in 1-2 concise sentences, preserving key information.", raw.String(), word),
	)
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

// cleanEtymologyText decodes HTML entities, strips residual tags, removes ad
// boilerplate, and collapses all whitespace so the LLM prompt stays compact
// without losing any of the source text's content.
func cleanEtymologyText(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRegexp.ReplaceAllString(s, " ")
	s = adNoiseRegexp.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// buildFamilyTree merges root-to-leaf lineages into a nested forest rooted at a
// synthetic "root" node. Each distinct top-level ancestor becomes a child of the
// root, and every node's value is the number of descendant leaves.
func buildFamilyTree(lineages [][]string) *familyNode {
	root := &familyNode{ID: "root", Name: "root"}
	for _, lineage := range lineages {
		if len(lineage) == 0 {
			continue
		}
		parent := root
		for _, name := range lineage {
			var child *familyNode
			for _, c := range parent.Children {
				if c.ID == name {
					child = c
					break
				}
			}
			if child == nil {
				child = &familyNode{ID: name, Name: familyDisplayName(name)}
				parent.Children = append(parent.Children, child)
			}
			parent = child
		}
	}
	setFamilyValues(root)
	return root
}

// setFamilyValues assigns each node a value equal to its leaf count.
func setFamilyValues(n *familyNode) int {
	if len(n.Children) == 0 {
		n.Value = 1
		return 1
	}
	total := 0
	for _, c := range n.Children {
		total += setFamilyValues(c)
	}
	n.Value = total
	return total
}

// familyDisplayName strips the bracketed glottocode suffix (e.g. " [indo1319]")
// from a Family name for display purposes.
func familyDisplayName(name string) string {
	if i := strings.Index(name, " ["); i >= 0 {
		return name[:i]
	}
	return name
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
	result, err := s.graph.ExecuteQuery(r.Context(), query,
		searchParams, neo4j.ExecuteQueryWithDatabase("neo4j"))
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
