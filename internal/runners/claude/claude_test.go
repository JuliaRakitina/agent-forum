package claude

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

func TestBuildInvocationSubscriptionSoftNoTools(t *testing.T) {
	inv, err := BuildInvocation(runner.Request{
		RunID:        "11111111-1111-1111-1111-111111111111",
		Prompt:       "question",
		Instructions: "be brief",
		Model:        "sonnet",
		Effort:       "high",
		Profile:      domain.ProfileSubscription,
		Tools:        domain.ToolsNone,
		Isolation:    domain.IsolationSoft,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--strict-mcp-config", "--setting-sources", "project",
		"--permission-prompts", "none",
		"--no-session-persistence",
		"--tools", "",
		"--model", "sonnet", "--effort", "high",
		"--append-system-prompt", "be brief",
	}
	if !slices.Equal(inv.Args, want) {
		t.Fatalf("args:\n got %q\nwant %q", inv.Args, want)
	}
	if inv.Env["CLAUDE_CODE_DISABLE_AUTO_MEMORY"] != "1" || inv.Stdin != "question" {
		t.Fatalf("env/stdin: %+v", inv)
	}
	for _, a := range inv.Args {
		if a == "--bare" {
			t.Fatal("subscription profile must not use --bare")
		}
	}
}

func TestBuildInvocationAPIKeyReadOnlyNoIsolation(t *testing.T) {
	inv, err := BuildInvocation(runner.Request{
		RunID:     "22222222-2222-2222-2222-222222222222",
		Profile:   domain.ProfileAPIKey,
		Tools:     domain.ToolsReadOnly,
		Isolation: domain.IsolationNone,
		Env:       map[string]string{"ANTHROPIC_API_KEY": "k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(inv.Args, " ")
	for _, want := range []string{"--bare", "--session-id 22222222-2222-2222-2222-222222222222", "--tools Read,Glob,Grep", `"blockReadsOutsideWorkingDirectories":true`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "--no-session-persistence") {
		t.Error("no isolation should keep the session")
	}
	if inv.Env["ANTHROPIC_API_KEY"] != "k" {
		t.Error("env must be passed through")
	}
}

func TestBuildInvocationResumeAndUnsupported(t *testing.T) {
	inv, err := BuildInvocation(runner.Request{Isolation: domain.IsolationNone, NativeSessionID: "abc"})
	if err != nil || !strings.Contains(strings.Join(inv.Args, " "), "--resume abc") {
		t.Fatalf("resume: %v %q", err, inv.Args)
	}
	cases := []runner.Request{
		{Isolation: domain.IsolationStrict},
		{Isolation: domain.IsolationSoft, NativeSessionID: "abc"},
		{Profile: "oauth-magic"},
		{Tools: "everything"},
	}
	for _, c := range cases {
		if _, err := BuildInvocation(c); !errors.Is(err, runner.ErrUnsupported) {
			t.Errorf("%+v: want ErrUnsupported, got %v", c, err)
		}
	}
}

const sampleStream = `{"type":"system","subtype":"init","session_id":"s-1","model":"claude-sonnet-5","tools":["Read"],"mcp_servers":[],"permissionMode":"default","apiKeySource":"none","claude_code_version":"2.1.300","cwd":"/tmp/run"}
{"type":"stream_event","session_id":"s-1","event":{"type":"message_start"}}
{"type":"stream_event","session_id":"s-1","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}
{"type":"stream_event","session_id":"s-1","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world"}}}
{"type":"assistant","session_id":"s-1","parent_tool_use_id":null,"message":{"role":"assistant","content":[{"type":"text","text":"Hello, world"},{"type":"tool_use","id":"tu-1","name":"Read","input":{"file_path":"/tmp/run/notes.md"}}]}}
{"type":"user","session_id":"s-1","parent_tool_use_id":null,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"notes","is_error":false}]}}
{"type":"system","subtype":"permission_denied","tool_name":"Bash","tool_use_id":"tu-2","message":"denied","session_id":"s-1"}
{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"retry_delay_ms":500,"error_status":529,"error":"overloaded","session_id":"s-1"}
not json at all
{"type":"result","subtype":"success","is_error":false,"duration_ms":10,"duration_api_ms":8,"num_turns":1,"result":"Hello, world","stop_reason":"end_turn","session_id":"s-1","total_cost_usd":0.0123,"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":3,"cache_creation_input_tokens":0},"permission_denials":[]}`

func TestParserStream(t *testing.T) {
	p := NewParser()
	var events []runner.Event
	for _, line := range strings.Split(sampleStream, "\n") {
		events = append(events, p.Feed(line)...)
	}
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, string(e.Kind)+":"+e.ToolStatus)
	}
	want := []string{"started:", "status:", "text_delta:", "text_delta:", "tool:started", "tool:completed", "tool:denied", "status:", "log:"}
	if !slices.Equal(kinds, want) {
		t.Fatalf("events:\n got %q\nwant %q", kinds, want)
	}
	if events[0].NativeSessionID != "s-1" || events[4].Tool != "Read" || events[4].Text != "/tmp/run/notes.md" || events[5].Tool != "Read" {
		t.Fatalf("unexpected events %+v", events)
	}
	res, errText, ok := p.Result()
	if !ok || errText != "" {
		t.Fatalf("result not ok: %q", errText)
	}
	if res.Text != "Hello, world" || res.Partial {
		t.Fatalf("text duplicated or partial: %+v", res)
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 || res.Usage.CachedTokens != 3 || res.Usage.CostUSD != 0.0123 || res.StopReason != "end_turn" || res.NativeSessionID != "s-1" {
		t.Fatalf("usage not mapped: %+v", res)
	}
}

func TestParserWithoutPartialMessagesUsesAssistantText(t *testing.T) {
	p := NewParser()
	p.Feed(`{"type":"assistant","parent_tool_use_id":null,"message":{"content":[{"type":"text","text":"Full answer"}]}}`)
	p.Feed(`{"type":"result","subtype":"success","is_error":false,"result":"Full answer","session_id":"s"}`)
	res, _, ok := p.Result()
	if !ok || res.Text != "Full answer" {
		t.Fatalf("%+v", res)
	}
}

func TestParserErrorResultAndMissingResult(t *testing.T) {
	p := NewParser()
	p.Feed(`{"type":"result","subtype":"error_max_turns","is_error":true,"result":"","errors":["too many turns"],"session_id":"s"}`)
	res, errText, ok := p.Result()
	if !ok || errText != "too many turns" || !res.Partial {
		t.Fatalf("%+v %q %v", res, errText, ok)
	}
	p2 := NewParser()
	p2.Feed(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"half"}}}`)
	res2, _, ok2 := p2.Result()
	if ok2 || res2.Text != "half" || !res2.Partial {
		t.Fatalf("%+v %v", res2, ok2)
	}
}
