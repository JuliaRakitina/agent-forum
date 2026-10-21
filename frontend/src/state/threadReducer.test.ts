import { describe, expect, it } from 'vitest'
import type { Event, Message, Round, Run, Thread, Topic } from '../api/types'
import { initialThreadState, threadReducer, type ThreadState } from './threadReducer'

const topic: Topic = { id: 't1', workspaceId: 'ws', title: 'T', kind: 'discussion', createdAt: '2026-10-21T10:00:00Z', updatedAt: '2026-10-21T10:00:00Z' }

function msg(id: string, extra: Partial<Message> = {}): Message {
  return { id, topicId: 't1', authorKind: 'human', body: `body ${id}`, visibility: 'visible', createdAt: `2026-10-21T10:00:0${id.length}Z`, ...extra }
}

function run(id: string, extra: Partial<Run> = {}): Run {
  return { id, topicId: 't1', participantId: 'p1', status: 'running', profile: 'subscription', tools: 'none', isolation: 'none', usage: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 }, createdAt: '2026-10-21T10:00:00Z', ...extra }
}

function round(id: string, extra: Partial<Round> = {}): Round {
  return { id, topicId: 't1', kind: 'single', status: 'running', prompt: 'p', participantIds: ['p1'], inputs: { messageIds: [] }, autoReveal: false, createdAt: '2026-10-21T10:00:00Z', ...extra }
}

function event(seq: number, type: string, payload: unknown, topicId = 't1'): Event {
  return { seq, type, topicId, at: '2026-10-21T10:01:00Z', payload }
}

function loaded(thread: Partial<Thread> = {}): ThreadState {
  const full: Thread = { topic, messages: [msg('m1')], runs: [], rounds: [], participants: [], lastEventSeq: 10, ...thread }
  const started = threadReducer(initialThreadState, { type: 'load/start', topicId: 't1' })
  return threadReducer(started, { type: 'load/success', thread: full })
}

describe('threadReducer', () => {
  it('loads a snapshot and ignores events already covered by it', () => {
    const s = loaded()
    expect(s.loading).toBe(false)
    expect(s.lastSeq).toBe(10)
    const same = threadReducer(s, { type: 'event', event: event(10, 'message.created', { message: msg('old') }) })
    expect(same.messages.map((m) => m.id)).toEqual(['m1'])
  })

  it('appends messages for the open topic only and keeps chronological order', () => {
    let s = loaded()
    s = threadReducer(s, { type: 'event', event: event(11, 'message.created', { message: msg('m3', { createdAt: '2026-10-21T10:00:09Z' }) }) })
    s = threadReducer(s, { type: 'event', event: event(12, 'message.created', { message: msg('m2', { createdAt: '2026-10-21T10:00:05Z' }) }) })
    s = threadReducer(s, { type: 'event', event: event(13, 'message.created', { message: msg('x', { topicId: 'other' }) }, 'other') })
    expect(s.messages.map((m) => m.id)).toEqual(['m1', 'm2', 'm3'])
    expect(s.lastSeq).toBe(13)
  })

  it('assembles streamed output in index order and tracks tool activity', () => {
    let s = loaded({ runs: [run('r1')] })
    s = threadReducer(s, { type: 'event', event: event(11, 'run.output', { runId: 'r1', index: 1, text: 'world', sealed: false }) })
    expect(s.streams.r1.text).toBe('')
    s = threadReducer(s, { type: 'event', event: event(12, 'run.output', { runId: 'r1', index: 0, text: 'hello ', sealed: false }) })
    expect(s.streams.r1.text).toBe('hello world')
    expect(s.streams.r1.nextIndex).toBe(2)
    s = threadReducer(s, { type: 'event', event: event(13, 'run.tool_activity', { runId: 'r1', tool: 'Read', summary: 'notes.md', status: 'completed' }) })
    expect(s.tools.r1).toBe('Read · completed · notes.md')
  })

  it('keeps sealed output and sealed messages hidden until the reveal', () => {
    let s = loaded({ rounds: [round('rd', { kind: 'independent', participantIds: ['p1', 'p2'] })] })
    s = threadReducer(s, { type: 'event', event: event(11, 'run.status_changed', { runId: 'r1', participantId: 'p1', roundId: 'rd', status: 'running' }) })
    expect(s.runs).toHaveLength(1)
    expect(s.streams.r1.sealed).toBe(true)
    s = threadReducer(s, { type: 'event', event: event(12, 'run.output', { runId: 'r1', index: 0, text: '', sealed: true }) })
    expect(s.streams.r1.text).toBe('')
    expect(s.streams.r1.chunks).toBe(1)
    s = threadReducer(s, { type: 'event', event: event(13, 'message.created', { message: msg('a1', { authorKind: 'agent', participantId: 'p1', runId: 'r1', roundId: 'rd', body: '', visibility: 'sealed' }) }) })
    s = threadReducer(s, { type: 'event', event: event(14, 'run.status_changed', { runId: 'r1', participantId: 'p1', roundId: 'rd', status: 'succeeded', usage: { inputTokens: 5, outputTokens: 2, cachedTokens: 0, costUsd: 0.1 } }) })
    expect(s.messages.find((m) => m.id === 'a1')?.visibility).toBe('sealed')
    expect(s.runs[0].status).toBe('succeeded')
    expect(s.runs[0].usage.costUsd).toBe(0.1)
    s = threadReducer(s, { type: 'event', event: event(15, 'round.status_changed', { roundId: 'rd', status: 'awaiting_reveal' }) })
    expect(s.rounds[0].status).toBe('awaiting_reveal')
    s = threadReducer(s, { type: 'event', event: event(16, 'message.revealed', { message: msg('a1', { authorKind: 'agent', participantId: 'p1', runId: 'r1', roundId: 'rd', body: 'the answer', visibility: 'visible' }) }) })
    s = threadReducer(s, { type: 'event', event: event(17, 'round.status_changed', { roundId: 'rd', status: 'revealed' }) })
    const revealed = s.messages.find((m) => m.id === 'a1')
    expect(revealed?.body).toBe('the answer')
    expect(revealed?.visibility).toBe('visible')
    expect(s.rounds[0].status).toBe('revealed')
    expect(s.rounds[0].revealedAt).toBeDefined()
    expect(s.resyncNeeded).toBe(false)
  })

  it('requests a resync on a sequence gap, an unknown round, or an explicit signal', () => {
    let s = loaded()
    s = threadReducer(s, { type: 'event', event: event(13, 'message.created', { message: msg('m9') }) })
    expect(s.resyncNeeded).toBe(true)
    expect(s.messages.map((m) => m.id)).toContain('m9')

    let t = loaded()
    t = threadReducer(t, { type: 'event', event: event(11, 'round.status_changed', { roundId: 'ghost', status: 'running' }) })
    expect(t.resyncNeeded).toBe(true)

    const u = threadReducer(loaded(), { type: 'resync/request' })
    expect(u.resyncNeeded).toBe(true)
  })

  it('buffers events while loading and replays those newer than the snapshot', () => {
    let s = threadReducer(initialThreadState, { type: 'load/start', topicId: 't1' })
    s = threadReducer(s, { type: 'event', event: event(10, 'message.created', { message: msg('stale') }) })
    s = threadReducer(s, { type: 'event', event: event(11, 'message.created', { message: msg('fresh') }) })
    expect(s.pending).toHaveLength(2)
    s = threadReducer(s, { type: 'load/success', thread: { topic, messages: [msg('m1')], runs: [], rounds: [], participants: [], lastEventSeq: 10 } })
    expect(s.messages.map((m) => m.id)).toEqual(['m1', 'fresh'])
    expect(s.pending).toHaveLength(0)
    expect(s.lastSeq).toBe(11)
  })

  it('ignores a snapshot for a topic that is no longer open', () => {
    const s = threadReducer(initialThreadState, { type: 'load/start', topicId: 't2' })
    const same = threadReducer(s, { type: 'load/success', thread: { topic, messages: [], runs: [], rounds: [], participants: [], lastEventSeq: 1 } })
    expect(same).toBe(s)
  })

  it('adopts accepted context revisions in version order', () => {
    let s = loaded()
    s = threadReducer(s, { type: 'event', event: event(11, 'context.revised', { revision: { id: 'c2', topicId: 't1', version: 2, status: 'accepted', authorKind: 'human', doc: { goal: 'g2' }, createdAt: '' } }) })
    expect(s.context?.version).toBe(2)
    s = threadReducer(s, { type: 'event', event: event(12, 'context.revised', { revision: { id: 'c1', topicId: 't1', version: 1, status: 'accepted', authorKind: 'human', doc: { goal: 'g1' }, createdAt: '' } }) })
    expect(s.context?.version).toBe(2)
    s = threadReducer(s, { type: 'event', event: event(13, 'context.revised', { revision: { id: 'c3', topicId: 't1', version: 3, status: 'proposed', authorKind: 'agent', doc: { goal: 'g3' }, createdAt: '' } }) })
    expect(s.context?.version).toBe(2)
  })
})
