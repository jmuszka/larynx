package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{}},
		{name: "single", in: "abc", want: []string{"abc"}},
		{name: "multiple", in: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "trims whitespace", in: " a , b ,c ", want: []string{"a", "b", "c"}},
		{name: "skips empties", in: "a,,b, ,c,", want: []string{"a", "b", "c"}},
		{name: "commas only", in: ",,,", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitCSV(tt.in))
		})
	}
}
