import { describe, expect, it } from 'vitest'
import { initialThreadState, type ThreadState } from '../state/threadReducer'
import { buildTimeline } from './timeline'

const base: ThreadState = {
  ...initialThreadState,
  topicId: 't1',
  messages: [
    { id: 'm1', topicId: 't1', authorKind: 'human', body: 'q', visibility: 'visible', createdAt: '2026-10-21T10:00:00Z' },
    { id: 'm2', topicId: 't1', authorKind: 'agent', participantId: 'p1', body: 'a', runId: 'r1', roundId: 'rd1', visibility: 'visible', createdAt: '2026-10-21T10:00:05Z' },
  ],
  runs: [
    { id: 'r1', topicId: 't1', participantId: 'p1', roundId: 'rd1', status: 'succeeded', profile: '', tools: '', isolation: '', usage: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 }, createdAt: '2026-10-21T10:00:01Z' },
    { id: 'r2', topicId: 't1', participantId: 'p2', roundId: 'rd2', status: 'running', profile: '', tools: '', isolation: '', usage: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 }, createdAt: '2026-10-21T10:00:11Z' },
    { id: 'r3', topicId: 't1', participantId: 'p1', roundId: 'rd2', status: 'failed', profile: '', tools: '', isolation: '', usage: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 }, createdAt: '2026-10-21T10:00:11Z' },
  ],
  rounds: [
    { id: 'rd1', topicId: 't1', kind: 'single', status: 'revealed', prompt: 'q', participantIds: ['p1'], inputs: { messageIds: [] }, autoReveal: false, createdAt: '2026-10-21T10:00:01Z' },
    { id: 'rd2', topicId: 't1', kind: 'independent', status: 'running', prompt: 'q2', participantIds: ['p1', 'p2'], inputs: { messageIds: [] }, autoReveal: false, createdAt: '2026-10-21T10:00:10Z' },
  ],
}

describe('buildTimeline', () => {
  it('orders items by time, hides finished single rounds and keeps runs without a message', () => {
    const items = buildTimeline(base)
    expect(items.map((i) => (i.kind === 'message' ? i.message.id : i.round.id))).toEqual(['m1', 'm2', 'rd2'])
    const round = items[2]
    if (round.kind !== 'round') throw new Error('expected round')
    expect(round.runs.map((r) => r.id)).toEqual(['r2', 'r3'])
  })
})
