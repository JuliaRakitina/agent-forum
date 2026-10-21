import { useCallback, useEffect, useState } from 'react'
import { parseEvent } from '../api/events'
import type { Capabilities, ForumApi, Participant, Topic, TopicKind, Workspace } from '../api/types'
import { errorMessage } from '../lib/time'

export interface ForumState {
  ready: boolean
  error: string | null
  workspace: Workspace | null
  topics: Topic[]
  participants: Participant[]
  capabilities: Capabilities[]
  selectedTopicId: string | null
}

const initial: ForumState = {
  ready: false,
  error: null,
  workspace: null,
  topics: [],
  participants: [],
  capabilities: [],
  selectedTopicId: null,
}

function sortTopics(topics: Topic[]): Topic[] {
  return [...topics].sort((a, b) => (a.updatedAt < b.updatedAt ? 1 : a.updatedAt > b.updatedAt ? -1 : 0))
}

function upsert<T extends { id: string }>(list: T[], item: T): T[] {
  const idx = list.findIndex((x) => x.id === item.id)
  if (idx < 0) return [...list, item]
  const copy = list.slice()
  copy[idx] = item
  return copy
}

/** Workspace-level state: topics, participants, capabilities and the selected topic. */
export function useForum(api: ForumApi | null) {
  const [state, setState] = useState<ForumState>(initial)

  useEffect(() => {
    if (!api) return
    let cancelled = false
    ;(async () => {
      try {
        const workspace = await api.ensureWorkspace()
        const [topics, participants, capabilities] = await Promise.all([
          api.listTopics(workspace.id),
          api.listParticipants(workspace.id),
          api.capabilities(),
        ])
        if (cancelled) return
        const sorted = sortTopics(topics)
        setState({
          ready: true,
          error: null,
          workspace,
          topics: sorted,
          participants,
          capabilities,
          selectedTopicId: sorted[0]?.id ?? null,
        })
      } catch (err) {
        if (!cancelled) setState((s) => ({ ...s, ready: true, error: errorMessage(err) }))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [api])

  useEffect(() => {
    if (!api) return
    return api.onEvent((ev) => {
      const parsed = parseEvent(ev)
      if (!parsed) return
      switch (parsed.type) {
        case 'topic.created': {
          const topic = parsed.payload.topic
          setState((s) => (s.workspace && topic.workspaceId === s.workspace.id ? { ...s, topics: sortTopics(upsert(s.topics, topic)) } : s))
          break
        }
        case 'participant.changed': {
          const participant = parsed.payload.participant
          setState((s) =>
            s.workspace && participant.workspaceId === s.workspace.id ? { ...s, participants: upsert(s.participants, participant) } : s,
          )
          break
        }
        case 'message.created': {
          const topicId = parsed.payload.message.topicId
          setState((s) => {
            const topic = s.topics.find((t) => t.id === topicId)
            if (!topic) return s
            return { ...s, topics: sortTopics(upsert(s.topics, { ...topic, updatedAt: ev.at })) }
          })
          break
        }
        default:
          break
      }
    })
  }, [api])

  const selectTopic = useCallback((topicId: string) => {
    setState((s) => ({ ...s, selectedTopicId: topicId }))
  }, [])

  const createTopic = useCallback(
    async (title: string, kind: TopicKind) => {
      if (!api || !state.workspace) throw new Error('workspace not ready')
      const topic = await api.createTopic(state.workspace.id, title, kind)
      setState((s) => ({ ...s, topics: sortTopics(upsert(s.topics, topic)), selectedTopicId: topic.id }))
      return topic
    },
    [api, state.workspace],
  )

  const saveParticipant = useCallback(
    async (participant: Participant) => {
      if (!api) throw new Error('backend not ready')
      const saved = await api.saveParticipant(participant)
      setState((s) => ({ ...s, participants: upsert(s.participants, saved) }))
      return saved
    },
    [api],
  )

  return { state, selectTopic, createTopic, saveParticipant }
}
