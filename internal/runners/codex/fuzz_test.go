package codex

import (
	"strings"
	"testing"
)

// FuzzParser feeds arbitrary JSONL-like input to the exec parser. It must
// not panic, a turn that never reached a terminal event must be partial,
// and a reported failure must never look like a complete answer.
//
//	go test -run='^$' -fuzz=FuzzParser -fuzztime=30s ./internal/runners/codex/
func FuzzParser(f *testing.F) {
	for _, line := range strings.Split(sampleJSONL, "\n") {
		f.Add(line)
	}
	f.Add(sampleJSONL)
	f.Add(`{"type":"item.completed","item":null}`)
	f.Add(`{"type":"item.updated","item":{"id":"a","type":"agent_message","text":"abc"}}` + "\n" + `{"type":"item.updated","item":{"id":"a","type":"agent_message","text":"ab"}}`)
	f.Add(`{"type":"turn.failed"}`)
	f.Add(`{"type":"turn.completed","usage":null}`)
	f.Add(`{"type":"item.completed","item":{"id":"x","type":"file_change","changes":[{"path":"` + strings.Repeat("p", 300) + `","kind":"add"}]}}`)

	f.Fuzz(func(t *testing.T, input string) {
		p := NewParser()
		for _, line := range strings.Split(input, "\n") {
			p.Feed(line)
		}
		res, errText, ok := p.Result()
		if !ok && !res.Partial {
			t.Fatalf("an unfinished turn must be partial: %+v", res)
		}
		if errText != "" && !res.Partial {
			t.Fatalf("a failed turn must be partial: %+v %q", res, errText)
		}
		_ = p.String()
	})
}
