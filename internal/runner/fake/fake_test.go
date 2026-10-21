package fake

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-forum/internal/runner"
)

type collector struct{ events []runner.Event }

func (c *collector) Emit(e runner.Event) { c.events = append(c.events, e) }

func (c *collector) text() string {
	var b strings.Builder
	for _, e := range c.events {
		if e.Kind == runner.EventTextDelta {
			b.WriteString(e.Text)
		}
	}
	return b.String()
}

func TestStreamsChunksAndReturnsFullText(t *testing.T) {
	r := New()
	r.Script("run-1", Script{Chunks: []string{"a", "b", "c"}, FailAfter: -1, Tool: "Read"})
	c := &collector{}
	res, err := r.Run(context.Background(), runner.Request{RunID: "run-1"}, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "abc" || res.Partial || c.text() != "abc" {
		t.Fatalf("unexpected result %+v, streamed %q", res, c.text())
	}
	if c.events[0].Kind != runner.EventStarted || c.events[1].Kind != runner.EventTool {
		t.Fatalf("unexpected event order: %+v", c.events)
	}
}

func TestFailAfterKeepsPartialText(t *testing.T) {
	r := New()
	r.Script("run-2", Script{Chunks: []string{"a", "b", "c"}, FailAfter: 2})
	res, err := r.Run(context.Background(), runner.Request{RunID: "run-2"}, &collector{})
	if !errors.Is(err, runner.ErrProcess) {
		t.Fatalf("want ErrProcess, got %v", err)
	}
	if res.Text != "ab" || !res.Partial {
		t.Fatalf("unexpected partial result %+v", res)
	}
}

func TestCancelAndTimeout(t *testing.T) {
	r := New()
	r.Script("hang", Script{Chunks: []string{"x"}, FailAfter: -1, Hang: true})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx, runner.Request{RunID: "hang"}, &collector{})
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, runner.ErrCancelled) {
		t.Fatalf("want ErrCancelled, got %v", err)
	}

	r.Script("slow", Script{Chunks: []string{"x", "y"}, Delay: 200 * time.Millisecond, FailAfter: -1})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	_, err := r.Run(ctx2, runner.Request{RunID: "slow"}, &collector{})
	if !errors.Is(err, runner.ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}
