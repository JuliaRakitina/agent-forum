import type { Message, Round, Run } from '../api/types'
import { isTerminalRun } from '../api/types'
import type { ThreadState } from '../state/threadReducer'

export type TimelineItem = { kind: 'message'; message: Message } | { kind: 'round'; round: Round; runs: Run[] }

function timeOf(item: TimelineItem): string {
  return item.kind === 'message' ? item.message.createdAt : item.round.createdAt
}

/**
 * Merges messages and rounds into one chronological list. A round shows its
 * active runs (and failed runs that produced no message); a finished "ask
 * one" round is omitted because its single answer speaks for itself.
 */
export function buildTimeline(state: ThreadState): TimelineItem[] {
  const messagesByRun = new Set(state.messages.filter((m) => m.runId).map((m) => m.runId))
  const items: TimelineItem[] = state.messages.map((message) => ({ kind: 'message', message }))
  for (const round of state.rounds) {
    if (round.kind === 'single' && round.status === 'revealed') continue
    const runs = state.runs.filter((r) => r.roundId === round.id && (!isTerminalRun(r.status) || !messagesByRun.has(r.id)))
    items.push({ kind: 'round', round, runs })
  }
  items.sort((a, b) => {
    const ta = timeOf(a)
    const tb = timeOf(b)
    if (ta !== tb) return ta < tb ? -1 : 1
    if (a.kind !== b.kind) return a.kind === 'message' ? -1 : 1
    return 0
  })
  return items
}
