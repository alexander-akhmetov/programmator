package llm

import (
	"bufio"
	"io"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/debug"
)

// ProcessTextOutput reads plain-text lines from r, calls opts.OnOutput for
// each line, and returns the accumulated output. Used by all executors in
// non-streaming mode.
func ProcessTextOutput(r io.Reader, opts InvokeOptions) string {
	var output strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text() + "\n"
		output.WriteString(line)
		if opts.OnOutput != nil {
			opts.OnOutput(line)
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Logf("stream: text scanner error: %v", err)
	}

	return output.String()
}
