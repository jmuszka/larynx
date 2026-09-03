package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderCypher(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		params map[string]any
		want   string
	}{
		{
			name:   "no params",
			query:  "MATCH (n) RETURN n",
			params: nil,
			want:   "MATCH (n) RETURN n",
		},
		{
			name:   "string param",
			query:  "MATCH (n) WHERE n.term = $term RETURN n",
			params: map[string]any{"term": "cat"},
			want:   "MATCH (n) WHERE n.term = 'cat' RETURN n",
		},
		{
			name:   "string slice param",
			query:  "UNWIND $langs AS l RETURN l",
			params: map[string]any{"langs": []string{"English", "French"}},
			want:   "UNWIND ['English', 'French'] AS l RETURN l",
		},
		{
			name:   "any slice param",
			query:  "UNWIND $vals AS v RETURN v",
			params: map[string]any{"vals": []any{"a", 1}},
			want:   "UNWIND ['a', 1] AS v RETURN v",
		},
		{
			name:   "int param",
			query:  "LIMIT $n",
			params: map[string]any{"n": 5},
			want:   "LIMIT 5",
		},
		{
			name:   "longer key replaced first",
			query:  "$word $wordX",
			params: map[string]any{"wordX": "x", "word": "w"},
			want:   "'w' 'x'",
		},
		{
			name:   "multiline flattened",
			query:  "MATCH (n)\nRETURN n",
			params: nil,
			want:   "MATCH (n) RETURN n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, renderCypher(tt.query, tt.params))
		})
	}
}

func TestFlatten(t *testing.T) {
	assert.Equal(t, "a b c", flatten("  a\n\n\tb \n c\n"))
	assert.Equal(t, "", flatten("\n \n"))
}

func TestRenderCypherValue(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{in: "s", want: "'s'"},
		{in: []string{"a", "b"}, want: "['a', 'b']"},
		{in: []any{"a", 2}, want: "['a', 2]"},
		{in: 42, want: "42"},
		{in: true, want: "true"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, renderCypherValue(tt.in))
	}
}

func TestRenderSQL(t *testing.T) {
	assert.Equal(t, "SELECT * FROM t WHERE a = 'x' AND b = 1",
		renderSQL("SELECT * FROM t WHERE a = ? AND b = ?", []any{"x", 1}))
}

func TestRenderSQLValue(t *testing.T) {
	assert.Equal(t, "'s'", renderSQLValue("s"))
	assert.Equal(t, "7", renderSQLValue(7))
}
