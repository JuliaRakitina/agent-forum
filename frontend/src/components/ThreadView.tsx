import { useEffect, useMemo, useRef, useState } from 'react'
import type { Participant } from '../api/types'
import { buildTimeline } from '../lib/timeline'
import type { ThreadState } from '../state/threadReducer'
import { MessageItem } from './MessageItem'
import { RoundBanner } from './RoundBanner'

interface Props {
  state: ThreadState
  participants: Participant[]
  onCancelRun: (runId: string) => void
  onCancelRound: (roundId: string) => void
  onReveal: (roundId: string) => void
  onRetry: (roundId: string) => void
}

const NEAR_BOTTOM_PX = 80

export function ThreadView({ state, participants, onCancelRun, onCancelRound, onReveal, onRetry }: Props) {
  const scroller = useRef<HTMLDivElement>(null)
  const [nearBottom, setNearBottom] = useState(true)
  const items = useMemo(() => buildTimeline(state), [state])

  function onScroll() {
    const el = scroller.current
    if (!el) return
    setNearBottom(el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX)
  }

  useEffect(() => {
    const el = scroller.current
    if (el && nearBottom) el.scrollTop = el.scrollHeight
  }, [items, nearBottom])

  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
    setNearBottom(true)
  }, [state.topicId])

  if (!state.topicId) {
    return (
      <div className="thread-empty">
        <p>Select a topic or create one to start a discussion.</p>
      </div>
    )
  }

  return (
    <div className="thread" ref={scroller} onScroll={onScroll} role="log" aria-live="polite">
      {state.loading && items.length === 0 && <p className="muted">Loading…</p>}
      {state.error && <p className="form-error">{state.error}</p>}
      {!state.loading && items.length === 0 && <p className="muted">No messages yet. Mention a participant below to ask for an answer.</p>}
      {items.map((item) =>
        item.kind === 'message' ? (
          <MessageItem key={item.message.id} message={item.message} participants={participants} />
        ) : (
          <RoundBanner
            key={item.round.id}
            round={item.round}
            runs={item.runs}
            participants={participants}
            streams={state.streams}
            tools={state.tools}
            onCancelRun={onCancelRun}
            onCancelRound={onCancelRound}
            onReveal={onReveal}
            onRetry={onRetry}
          />
        ),
      )}
    </div>
  )
}
