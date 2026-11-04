package claude

import (
	"strings"
	"testing"

	"github.com/JuliaRakitina/agent-forum/internal/runner"
)

// FuzzParser feeds arbitrary stream-json-like input to the parser. The CLI
// output is external input: whatever it contains, the parser must not
// panic, and the answer it accumulates must be exactly the text it emitted
// as deltas, so the UI and the stored message never disagree.
//
//	go test -run='^$' -fuzz=FuzzParser -fuzztime=30s ./internal/runners/claude/
func FuzzParser(f *testing.F) {
	for _, line := range strings.Split(sampleStream, "\n") {
		f.Add(line)
	}
	f.Add(sampleStream)
	f.Add(`{"type":"assistant","message":{"content":[{"type":"text","text":"a"},{"type":"tool_use","id":"t","name":"Read","input":null}]}}`)
	f.Add(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"missing"}]}}`)
	f.Add(`{"type":"result","subtype":"success","stop_reason":null,"usage":{}}`)
	f.Add(`{"type":"system","subtype":"init","mcp_servers":[{"name":1}]}`)
	f.Add("{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"\\u0000\"}}}")

	f.Fuzz(func(t *testing.T, input string) {
		p := NewParser()
		var streamed strings.Builder
		for _, line := range strings.Split(input, "\n") {
			for _, ev := range p.Feed(line) {
				if ev.Kind == runner.EventTextDelta {
					streamed.WriteString(ev.Text)
				}
			}
		}
		if got := p.Text.String(); got != streamed.String() {
			t.Fatalf("accumulated text %q differs from streamed deltas %q", got, streamed.String())
		}
		res, errText, ok := p.Result()
		if !ok && !res.Partial {
			t.Fatalf("a run without a result message must be partial: %+v", res)
		}
		if errText != "" && !res.Partial {
			t.Fatalf("an error result must be partial: %+v %q", res, errText)
		}
	})
}
