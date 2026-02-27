package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnsiHelpers(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		expected string
	}{
		{"ansi bold code", func() string { return ansi(1, "hello") }, "\033[1mhello\033[0m"},
		{"ansi dim code", func() string { return ansi(2, "world") }, "\033[2mworld\033[0m"},
		{"bold", func() string { return bold("test") }, "\033[1mtest\033[0m"},
		{"dim", func() string { return dim("test") }, "\033[2mtest\033[0m"},
		{"fg", func() string { return fg(42, "test") }, "\033[38;5;42mtest\033[0m"},
		{"fgBold", func() string { return fgBold(196, "test") }, "\033[1;38;5;196mtest\033[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.fn())
		})
	}
}
