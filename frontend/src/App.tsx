import { useCallback, useEffect, useState } from 'react'
import { createApi, type ForumApi } from './api'
import type { ContextDoc, Participant, PostInput } from './api/types'
import { Composer } from './components/Composer'
import { RightPanel } from './components/RightPanel'
import { Sidebar } from './components/Sidebar'
import { ThreadView } from './components/ThreadView'
import { TopBar } from './components/TopBar'
import { errorMessage } from './lib/time'
import { useForum } from './state/useForum'
import { useThread } from './state/useThread'

export default function App() {
  const [api, setApi] = useState<ForumApi | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    void createApi().then((a) => {
      if (active) setApi(a)
    })
    return () => {
      active = false
    }
  }, [])

  const forum = useForum(api)
  const thread = useThread(api, forum.state.selectedTopicId)

  const run = useCallback(async (fn: () => Promise<unknown>) => {
    try {
      await fn()
      setNotice(null)
    } catch (err) {
      setNotice(errorMessage(err))
    }
  }, [])

  const onSend = useCallback(
    async (input: PostInput) => {
      if (!api) throw new Error('backend not ready')
      const result = await api.postMessage(input)
      thread.dispatch({ type: 'message/add', message: result.message })
      if (result.round) thread.dispatch({ type: 'round/add', round: result.round })
    },
    [api, thread],
  )

  const onSaveContext = useCallback(
    async (doc: ContextDoc) => {
      if (!api || !forum.state.selectedTopicId) throw new Error('no topic selected')
      return api.saveContext(forum.state.selectedTopicId, doc)
    },
    [api, forum.state.selectedTopicId],
  )

  const onSaveParticipant = useCallback((p: Participant) => forum.saveParticipant(p), [forum])

  if (!api || !forum.state.ready) {
    return <div className="boot">Starting Agent Forum…</div>
  }
  if (!forum.state.workspace) {
    return <div className="boot form-error">{forum.state.error ?? 'No workspace available.'}</div>
  }

  const topic = forum.state.topics.find((t) => t.id === forum.state.selectedTopicId) ?? null

  return (
    <div className="app">
      <TopBar workspaceName={forum.state.workspace.name} topicTitle={topic?.title ?? null} mode={api.mode} />
      <Sidebar
        workspace={forum.state.workspace}
        topics={forum.state.topics}
        selectedTopicId={forum.state.selectedTopicId}
        participants={forum.state.participants}
        onSelectTopic={forum.selectTopic}
        onCreateTopic={forum.createTopic}
      />
      <main className="main">
        {(notice || forum.state.error) && (
          <div className="notice" role="alert">
            <span>{notice ?? forum.state.error}</span>
            <button type="button" className="btn btn-small btn-ghost" onClick={() => setNotice(null)}>
              Dismiss
            </button>
          </div>
        )}
        <ThreadView
          state={thread.state}
          participants={forum.state.participants}
          onCancelRun={(id) => void run(() => api.cancelRun(id))}
          onCancelRound={(id) => void run(() => api.cancelRound(id))}
          onReveal={(id) => void run(() => api.revealRound(id))}
          onRetry={(id) => void run(() => api.retryRound(id))}
        />
        <Composer topicId={forum.state.selectedTopicId} participants={forum.state.participants} onSend={onSend} />
      </main>
      <RightPanel
        workspace={forum.state.workspace}
        topicId={forum.state.selectedTopicId}
        context={thread.state.context}
        participants={forum.state.participants}
        capabilities={forum.state.capabilities}
        onSaveContext={onSaveContext}
        onSaveParticipant={onSaveParticipant}
      />
    </div>
  )
}
