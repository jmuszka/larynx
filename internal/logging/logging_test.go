package logging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want Level
	}{
		{in: "debug", want: LevelDebug},
		{in: "info", want: LevelInfo},
		{in: "warn", want: LevelWarn},
		{in: "error", want: LevelError},
		{in: "fatal", want: LevelFatal},
		{in: "unknown", want: LevelInfo},
		{in: "", want: LevelInfo},
		{in: "INFO", want: LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseLevel(tt.in))
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("no file path", func(t *testing.T) {
		s, err := New(Config{Level: LevelInfo})
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.NotNil(t, s.Logger())
		s.Close()
	})

	t.Run("creates file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.log")
		s, err := New(Config{Level: LevelInfo, FilePath: path})
		require.NoError(t, err)
		s.Info("hello")
		s.Close()

		_, err = os.Stat(path)
		assert.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "hello")
	})

	t.Run("invalid path", func(t *testing.T) {
		_, err := New(Config{Level: LevelInfo, FilePath: filepath.Join(t.TempDir(), "missing", "x.log")})
		assert.Error(t, err)
	})
}
