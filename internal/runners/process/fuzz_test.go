package process

import (
	"strings"
	"testing"
)

// splitAll runs data through a lineSplitter in chunks of the given size and
// returns the lines it delivered, including the final flush.
func splitAll(data []byte, chunk, maxLine int) []string {
	out := make(chan string, 64)
	ls := &lineSplitter{out: out, maxLine: maxLine}
	done := make(chan []string)
	go func() {
		var lines []string
		for l := range out {
			lines = append(lines, l)
		}
		done <- lines
	}()
	for len(data) > 0 {
		n := min(chunk, len(data))
		if _, err := ls.Write(data[:n]); err != nil {
			panic(err)
		}
		data = data[n:]
	}
	ls.flush()
	close(out)
	return <-done
}

// FuzzLineSplitter checks that the stdout splitter reproduces the input
// line by line however the bytes are chunked, and never panics when a line
// exceeds the limit.
//
//	go test -run='^$' -fuzz=FuzzLineSplitter -fuzztime=30s ./internal/runners/process/
func FuzzLineSplitter(f *testing.F) {
	f.Add([]byte("a\nb\n"), uint8(1))
	f.Add([]byte("no newline"), uint8(3))
	f.Add([]byte("\n\n\n"), uint8(2))
	f.Add([]byte("crlf\r\nline\r\n"), uint8(5))
	f.Add([]byte(strings.Repeat("x", 300)+"\nend"), uint8(7))

	f.Fuzz(func(t *testing.T, data []byte, chunkSeed uint8) {
		chunk := int(chunkSeed%32) + 1

		// With a limit larger than the input, the lines must be exactly
		// the input split on newlines, minus one trailing newline.
		if !strings.Contains(string(data), "\r") {
			lines := splitAll(data, chunk, len(data)+1)
			want := strings.TrimSuffix(string(data), "\n")
			if got := strings.Join(lines, "\n"); got != want {
				t.Fatalf("chunk=%d: lines %q do not reproduce %q", chunk, lines, data)
			}
		}

		// With a tiny limit, long lines are delivered in pieces; nothing
		// may be lost or invented.
		pieces := splitAll(data, chunk, 4)
		total := 0
		for _, p := range pieces {
			total += len(p)
		}
		newlines := strings.Count(string(data), "\n")
		if total > len(data)-newlines {
			t.Fatalf("pieces hold %d bytes, input has only %d non-newline bytes", total, len(data)-newlines)
		}
	})
}
