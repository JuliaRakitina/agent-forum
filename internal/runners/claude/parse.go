package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

// Parser turns stream-json lines into runner events. Message shapes follow
// the Agent SDK type declarations (@anthropic-ai/claude-agent-sdk 0.3.306,
// sdk.d.ts): system/init, stream_event, assistant, user, system/
// permission_denied, system/api_retry and result. Unknown lines are ignored
// so a newer CLI does not break the adapter.
type Parser struct {
	final      *resultMessage
	sawDelta   bool
	toolNames  map[string]string
	textBlocks int
	// Text accumulates the answer from deltas, or from assistant messages
	// when the CLI sent no deltas.
	Text strings.Builder
}

// NewParser returns an empty parser.
func NewParser() *Parser {
	return &Parser{toolNames: map[string]string{}}
}

type envelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
}

type initMessage struct {
	Model      string   `json:"model"`
	Tools      []string `json:"tools"`
	MCPServers []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"mcp_servers"`
	PermissionMode string `json:"permissionMode"`
	APIKeySource   string `json:"apiKeySource"`
	Version        string `json:"claude_code_version"`
}

type streamEvent struct {
	ParentToolUseID *string `json:"parent_tool_use_id"`
	Event           struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

type apiMessage struct {
	Message struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
}

type permissionDenied struct {
	ToolName string `json:"tool_name"`
	Message  string `json:"message"`
}

type apiRetry struct {
	Attempt    int    `json:"attempt"`
	MaxRetries int    `json:"max_retries"`
	Error      string `json:"error"`
}

type resultMessage struct {
	Subtype    string   `json:"subtype"`
	IsError    bool     `json:"is_error"`
	Result     string   `json:"result"`
	StopReason *string  `json:"stop_reason"`
	NumTurns   int      `json:"num_turns"`
	TotalCost  float64  `json:"total_cost_usd"`
	SessionID  string   `json:"session_id"`
	Errors     []string `json:"errors"`
	Usage      struct {
		InputTokens         int64 `json:"input_tokens"`
		OutputTokens        int64 `json:"output_tokens"`
		CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	PermissionDenials []struct {
		ToolName string `json:"tool_name"`
	} `json:"permission_denials"`
}

// Feed parses one line. It returns the events to emit; a line that is not
// JSON or not a known message yields no events and no error.
func (p *Parser) Feed(line string) []runner.Event {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		if line != "" {
			return []runner.Event{{Kind: runner.EventLog, Text: line}}
		}
		return nil
	}
	var env envelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return []runner.Event{{Kind: runner.EventLog, Text: line}}
	}
	switch env.Type {
	case "system":
		return p.system(env, line)
	case "stream_event":
		var ev streamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil
		}
		if ev.ParentToolUseID != nil && *ev.ParentToolUseID != "" {
			return nil // subagent traffic is not part of the answer
		}
		if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" && ev.Event.Delta.Text != "" {
			p.sawDelta = true
			p.Text.WriteString(ev.Event.Delta.Text)
			return []runner.Event{{Kind: runner.EventTextDelta, Text: ev.Event.Delta.Text}}
		}
		return nil
	case "assistant":
		return p.assistant(line)
	case "user":
		return p.user(line)
	case "result":
		var res resultMessage
		if err := json.Unmarshal([]byte(line), &res); err != nil {
			return nil
		}
		p.final = &res
		return nil
	}
	return nil
}

func (p *Parser) system(env envelope, line string) []runner.Event {
	switch env.Subtype {
	case "init":
		var init initMessage
		_ = json.Unmarshal([]byte(line), &init)
		mcp := make([]string, 0, len(init.MCPServers))
		for _, s := range init.MCPServers {
			mcp = append(mcp, s.Name+":"+s.Status)
		}
		return []runner.Event{
			{Kind: runner.EventStarted, NativeSessionID: env.SessionID},
			{Kind: runner.EventStatus, Text: fmt.Sprintf("claude %s model=%s mode=%s auth=%s tools=[%s] mcp=[%s]",
				init.Version, init.Model, init.PermissionMode, init.APIKeySource,
				strings.Join(init.Tools, ","), strings.Join(mcp, ","))},
		}
	case "permission_denied":
		var d permissionDenied
		_ = json.Unmarshal([]byte(line), &d)
		return []runner.Event{{Kind: runner.EventTool, Tool: d.ToolName, ToolStatus: "denied", Text: d.Message}}
	case "api_retry":
		var r apiRetry
		_ = json.Unmarshal([]byte(line), &r)
		return []runner.Event{{Kind: runner.EventStatus, Text: fmt.Sprintf("api retry %d/%d: %s", r.Attempt, r.MaxRetries, r.Error)}}
	}
	return nil
}

func (p *Parser) assistant(line string) []runner.Event {
	var msg apiMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}
	if msg.ParentToolUseID != nil && *msg.ParentToolUseID != "" {
		return nil // subagent traffic is not part of the answer
	}
	var events []runner.Event
	for _, block := range msg.Message.Content {
		switch block.Type {
		case "text":
			p.textBlocks++
			if !p.sawDelta && block.Text != "" {
				// No partial messages were streamed: take the full text.
				p.Text.WriteString(block.Text)
				events = append(events, runner.Event{Kind: runner.EventTextDelta, Text: block.Text})
			}
		case "tool_use":
			p.toolNames[block.ID] = block.Name
			events = append(events, runner.Event{Kind: runner.EventTool, Tool: block.Name, ToolStatus: "started", Text: summarizeInput(block.Input)})
		}
	}
	return events
}

func (p *Parser) user(line string) []runner.Event {
	var msg apiMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}
	var events []runner.Event
	for _, block := range msg.Message.Content {
		if block.Type != "tool_result" {
			continue
		}
		status := "completed"
		if block.IsError {
			status = "failed"
		}
		events = append(events, runner.Event{Kind: runner.EventTool, Tool: p.toolNames[block.ToolUseID], ToolStatus: status})
	}
	return events
}

// summarizeInput renders a short description of a tool call for the UI.
func summarizeInput(raw json.RawMessage) string {
	var in map[string]any
	if err := json.Unmarshal(raw, &in); err != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "pattern", "path", "query", "url", "description"} {
		if v, ok := in[key].(string); ok && v != "" {
			return truncate(v, 120)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Result assembles the run result from what was parsed. ok is false when no
// result message was seen, which means the process ended early.
func (p *Parser) Result() (res runner.Result, errText string, ok bool) {
	res.Text = p.Text.String()
	if p.final == nil {
		res.Partial = true
		return res, "", false
	}
	f := p.final
	res.NativeSessionID = f.SessionID
	res.Usage = domain.Usage{
		InputTokens:  f.Usage.InputTokens,
		OutputTokens: f.Usage.OutputTokens,
		CachedTokens: f.Usage.CacheReadTokens,
		CostUSD:      f.TotalCost,
	}
	if f.StopReason != nil {
		res.StopReason = *f.StopReason
	}
	if f.Subtype == "success" && !f.IsError {
		if res.Text == "" {
			res.Text = f.Result
		}
		return res, "", true
	}
	res.Partial = true
	msg := f.Subtype
	if f.IsError && f.Result != "" {
		msg = f.Result
	} else if len(f.Errors) > 0 {
		msg = strings.Join(f.Errors, "; ")
	}
	return res, msg, true
}
