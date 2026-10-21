package codex

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

func TestBuildInvocationSoftReadOnly(t *testing.T) {
	inv, err := BuildInvocation(runner.Request{
		Prompt:       "question",
		Instructions: "be brief",
		Model:        "gpt-5-codex",
		Effort:       "high",
		WorkDir:      "/tmp/run",
		Profile:      domain.ProfileSubscription,
		Tools:        domain.ToolsNone,
		Isolation:    domain.IsolationSoft,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"exec", "--json", "--skip-git-repo-check", "-C", "/tmp/run",
		"--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--sandbox", "read-only",
		"-m", "gpt-5-codex", "-c", `model_reasoning_effort="high"`,
		"-",
	}
	if !slices.Equal(inv.Args, want) {
		t.Fatalf("args:\n got %q\nwant %q", inv.Args, want)
	}
	if !strings.HasPrefix(inv.Stdin, "## Instructions\n\nbe brief\n\n## Task\n\nquestion") {
		t.Fatalf("stdin: %q", inv.Stdin)
	}
}

func TestBuildInvocationWorkspaceWriteResumeAndErrors(t *testing.T) {
	inv, err := BuildInvocation(runner.Request{Tools: domain.ToolsWorkspaceWrite, Isolation: domain.IsolationNone, NativeSessionID: "thr-1"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(inv.Args, " ")
	if !strings.Contains(joined, "--sandbox workspace-write") || !strings.HasSuffix(joined, "resume thr-1 -") || strings.Contains(joined, "--ephemeral") {
		t.Fatalf("args: %q", joined)
	}
	cases := []runner.Request{
		{Profile: domain.ProfileAPIKey},
		{Isolation: domain.IsolationStrict},
		{Isolation: domain.IsolationSoft, NativeSessionID: "x"},
		{Tools: "nope"},
	}
	for _, c := range cases {
		if _, err := BuildInvocation(c); !errors.Is(err, runner.ErrUnsupported) {
			t.Errorf("%+v: want ErrUnsupported, got %v", c, err)
		}
	}
	if _, err := BuildInvocation(runner.Request{Profile: domain.ProfileAPIKey, Env: map[string]string{"CODEX_API_KEY": "k"}}); err != nil {
		t.Errorf("api key profile with key: %v", err)
	}
}

const sampleJSONL = `{"type":"thread.started","thread_id":"thr-9"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"i1","type":"reasoning","text":"thinking"}}
{"type":"item.started","item":{"id":"i2","type":"command_execution","command":"ls -la","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.completed","item":{"id":"i2","type":"command_execution","command":"ls -la","aggregated_output":"a\nb","exit_code":0,"status":"completed"}}
{"type":"item.updated","item":{"id":"i3","type":"agent_message","text":"Hello"}}
{"type":"item.completed","item":{"id":"i3","type":"agent_message","text":"Hello, world"}}
{"type":"item.completed","item":{"id":"i4","type":"file_change","changes":[{"path":"a.txt","kind":"add"}],"status":"completed"}}
{"type":"item.completed","item":{"id":"i5","type":"agent_message","text":"Second paragraph"}}
plain stderr-like noise on stdout
{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":20,"reasoning_output_tokens":5}}`

func TestParserStream(t *testing.T) {
	p := NewParser()
	var events []runner.Event
	for _, line := range strings.Split(sampleJSONL, "\n") {
		events = append(events, p.Feed(line)...)
	}
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, string(e.Kind)+":"+e.ToolStatus+":"+e.Text)
	}
	want := []string{
		"started::",
		"tool:started:ls -la",
		"tool:completed:ls -la",
		"text_delta::Hello",
		"text_delta::, world",
		"tool:completed:add a.txt",
		"text_delta::Second paragraph",
		"log::plain stderr-like noise on stdout",
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("events:\n got %q\nwant %q", kinds, want)
	}
	res, errText, ok := p.Result()
	if !ok || errText != "" || res.Partial {
		t.Fatalf("result: %+v %q %v", res, errText, ok)
	}
	if res.Text != "Hello, world\n\nSecond paragraph" || res.NativeSessionID != "thr-9" {
		t.Fatalf("text/thread: %+v", res)
	}
	if res.Usage.InputTokens != 100 || res.Usage.CachedTokens != 40 || res.Usage.OutputTokens != 20 {
		t.Fatalf("usage: %+v", res.Usage)
	}
}

func TestParserFailureAndEarlyExit(t *testing.T) {
	p := NewParser()
	p.Feed(`{"type":"thread.started","thread_id":"t"}`)
	p.Feed(`{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"partial"}}`)
	p.Feed(`{"type":"turn.failed","error":{"message":"rate limited"}}`)
	res, errText, ok := p.Result()
	if !ok || errText != "rate limited" || !res.Partial || res.Text != "partial" {
		t.Fatalf("%+v %q %v", res, errText, ok)
	}
	p2 := NewParser()
	p2.Feed(`{"type":"error","message":"boom"}`)
	res2, errText2, ok2 := p2.Result()
	if ok2 || errText2 != "boom" || !res2.Partial {
		t.Fatalf("%+v %q %v", res2, errText2, ok2)
	}
}

func TestTextDelta(t *testing.T) {
	if d := textDelta("", "abc"); d != "abc" {
		t.Fatal(d)
	}
	if d := textDelta("abc", "abcdef"); d != "def" {
		t.Fatal(d)
	}
	if d := textDelta("abc", "abc"); d != "" {
		t.Fatal(d)
	}
	if d := textDelta("abc", "xyz"); d != "\nxyz" {
		t.Fatal(d)
	}
}
