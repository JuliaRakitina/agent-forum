package orchestrator

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

// streamer coalesces text deltas into chunks that are persisted and
// published together. It flushes when the buffer reaches FlushBytes, when
// FlushInterval has passed since the first unflushed byte, and on demand.
type streamer struct {
	svc      *Service
	runID    domain.ID
	topicID  domain.ID
	sealed   bool
	interval time.Duration
	maxBytes int

	mu    sync.Mutex
	buf   strings.Builder
	index int
	timer *time.Timer
	// flushMu is held from index assignment through the commit, so chunks
	// are committed in index order even when a timer flush and a size
	// flush race, and a final flush waits for an in-flight one.
	flushMu sync.Mutex
}

func newStreamer(svc *Service, runID, topicID domain.ID, sealed bool, interval time.Duration, maxBytes int) *streamer {
	return &streamer{svc: svc, runID: runID, topicID: topicID, sealed: sealed, interval: interval, maxBytes: maxBytes}
}

func (st *streamer) write(text string) {
	if text == "" {
		return
	}
	st.mu.Lock()
	st.buf.WriteString(text)
	if st.buf.Len() >= st.maxBytes {
		st.mu.Unlock()
		st.flush()
		return
	}
	if st.timer == nil {
		st.timer = time.AfterFunc(st.interval, st.flush)
	}
	st.mu.Unlock()
}

// flush persists and publishes the buffered text as one chunk.
func (st *streamer) flush() {
	st.flushMu.Lock()
	defer st.flushMu.Unlock()
	st.mu.Lock()
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
	if st.buf.Len() == 0 {
		st.mu.Unlock()
		return
	}
	text := st.buf.String()
	st.buf.Reset()
	index := st.index
	st.index++
	st.mu.Unlock()

	_ = st.svc.Commit(context.Background(), func(tx *storage.Tx, emit func(domain.Event)) error {
		if err := tx.AppendRunOutput(st.runID, index, text); err != nil {
			return err
		}
		payload := domain.RunOutputPayload{RunID: st.runID, Index: index, Text: text, Sealed: st.sealed}
		if st.sealed {
			payload.Text = ""
		}
		emit(domain.NewEvent(domain.EventRunOutput, st.topicID, payload))
		return nil
	})
}
