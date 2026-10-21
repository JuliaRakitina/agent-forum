package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-forum/internal/domain"
	"agent-forum/internal/runner"
)

// Parser turns `codex exec --json` lines into runner events. The wire format
// is defined in codex-rs/exec/src/exec_events.rs: a "type"-tagged envelope
// (thread.started, turn.started, turn.completed, turn.failed, item.started,
// item.updated, item.completed, error) whose items are flat objects with
// "id" and "type" (agent_message, reasoning, command_execution, file_change,
// mcp_tool_call, web_search, todo_list, error).
type Parser struct {
	threadID string
	// texts holds the latest text of every agent_message item in order of
	// first appearance, so repeated item.updated events are turned into
	// deltas and the final answer is the concatenation.
	order    []string
	texts    map[string]string
	usage    domain.Usage
	sawTurn  bool
	finished bool
	failure  string
}

// NewParser returns an empty parser.
func NewParser() *Parser { return &Parser{texts: map[string]string{}} }

type envelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
	Usage    *struct {
		InputTokens       int64 `json:"input_tokens"`
		CachedInputTokens int64 `json:"cached_input_tokens"`
		OutputTokens      int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

type item struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Text             string `json:"text"`
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code"`
	Status           string `json:"status"`
	Server           string `json:"server"`
	Tool             string `json:"tool"`
	Query            string `json:"query"`
	Message          string `json:"message"`
	Changes          []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`
}

// Feed parses one line and returns the events to emit.
func (p *Parser) Feed(line string) []runner.Event {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if line[0] != '{' {
		return []runner.Event{{Kind: runner.EventLog, Text: line}}
	}
	var env envelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return []runner.Event{{Kind: runner.EventLog, Text: line}}
	}
	switch env.Type {
	case "thread.started":
		p.threadID = env.ThreadID
		return []runner.Event{{Kind: runner.EventStarted, NativeSessionID: env.ThreadID}}
	case "turn.started":
		p.sawTurn = true
		return nil
	case "turn.completed":
		p.finished = true
		p.failure = "" // an earlier error event was recovered from
		if env.Usage != nil {
			p.usage = domain.Usage{
				InputTokens:  env.Usage.InputTokens,
				OutputTokens: env.Usage.OutputTokens,
				CachedTokens: env.Usage.CachedInputTokens,
			}
		}
		return nil
	case "turn.failed":
		p.finished = true
		if env.Error != nil {
			p.failure = env.Error.Message
		} else {
			p.failure = "turn failed"
		}
		return []runner.Event{{Kind: runner.EventStatus, Text: "turn failed: " + p.failure}}
	case "error":
		if p.failure == "" {
			p.failure = env.Message
		}
		return []runner.Event{{Kind: runner.EventStatus, Text: "error: " + env.Message}}
	case "item.started", "item.updated", "item.completed":
		var it item
		if err := json.Unmarshal(env.Item, &it); err != nil {
			return nil
		}
		return p.item(env.Type, it)
	}
	return nil
}

func (p *Parser) item(eventType string, it item) []runner.Event {
	switch it.Type {
	case "agent_message":
		prev, seen := p.texts[it.ID]
		if !seen {
			p.order = append(p.order, it.ID)
		}
		p.texts[it.ID] = it.Text
		delta := textDelta(prev, it.Text)
		if delta == "" {
			return nil
		}
		return []runner.Event{{Kind: runner.EventTextDelta, Text: delta}}
	case "reasoning", "todo_list":
		return nil
	case "command_execution":
		return []runner.Event{{Kind: runner.EventTool, Tool: "shell", ToolStatus: toolStatus(eventType, it.Status), Text: truncate(it.Command, 120)}}
	case "file_change":
		paths := make([]string, 0, len(it.Changes))
		for _, c := range it.Changes {
			paths = append(paths, c.Kind+" "+c.Path)
		}
		return []runner.Event{{Kind: runner.EventTool, Tool: "file_change", ToolStatus: toolStatus(eventType, it.Status), Text: truncate(strings.Join(paths, ", "), 120)}}
	case "mcp_tool_call":
		return []runner.Event{{Kind: runner.EventTool, Tool: it.Server + "." + it.Tool, ToolStatus: toolStatus(eventType, it.Status), Text: ""}}
	case "web_search":
		return []runner.Event{{Kind: runner.EventTool, Tool: "web_search", ToolStatus: toolStatus(eventType, ""), Text: truncate(it.Query, 120)}}
	case "error":
		return []runner.Event{{Kind: runner.EventStatus, Text: "error item: " + it.Message}}
	}
	return nil
}

// textDelta returns what was appended to prev to produce next. When next
// does not extend prev (the item was rewritten), the whole new text is
// returned with a separator so nothing is lost.
func textDelta(prev, next string) string {
	if prev == "" {
		return next
	}
	if strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	if next == prev {
		return ""
	}
	return "\n" + next
}

func toolStatus(eventType, status string) string {
	switch status {
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	case "declined":
		return "denied"
	}
	if eventType == "item.completed" {
		return "completed"
	}
	return "started"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Result assembles the run result. ok reports whether the turn reached a
// terminal event (turn.completed or turn.failed).
func (p *Parser) Result() (res runner.Result, errText string, ok bool) {
	parts := make([]string, 0, len(p.order))
	for _, id := range p.order {
		if t := p.texts[id]; t != "" {
			parts = append(parts, t)
		}
	}
	res.Text = strings.Join(parts, "\n\n")
	res.NativeSessionID = p.threadID
	res.Usage = p.usage
	if !p.finished {
		res.Partial = true
		return res, p.failure, false
	}
	if p.failure != "" {
		res.Partial = true
		return res, p.failure, true
	}
	res.StopReason = "turn.completed"
	return res, "", true
}

// String describes the parser state for diagnostics.
func (p *Parser) String() string {
	return fmt.Sprintf("codex parser: thread=%s items=%d finished=%v failure=%q", p.threadID, len(p.order), p.finished, p.failure)
}
