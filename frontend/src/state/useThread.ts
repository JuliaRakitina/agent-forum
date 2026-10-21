import { useCallback, useEffect, useReducer } from 'react'
import type { ForumApi } from '../api/types'
import { errorMessage } from '../lib/time'
import { initialThreadState, threadReducer } from './threadReducer'

const PAGE_SIZE = 200

/** Loads a topic, keeps it current through events and reloads on resync. */
export function useThread(api: ForumApi | null, topicId: string | null) {
  const [state, dispatch] = useReducer(threadReducer, initialThreadState)

  const load = useCallback(async () => {
    if (!api || !topicId) return
    dispatch({ type: 'load/start', topicId })
    try {
      const thread = await api.getThread(topicId, PAGE_SIZE)
      const sealedRounds = new Set(thread.rounds.filter((r) => r.kind === 'independent' && r.status !== 'revealed').map((r) => r.id))
      const outputs: Record<string, string> = {}
      await Promise.all(
        thread.runs
          .filter((r) => r.status === 'running' && !(r.roundId && sealedRounds.has(r.roundId)))
          .map(async (r) => {
            try {
              outputs[r.id] = await api.runOutput(r.id)
            } catch {
              // A run that finished meanwhile has its text in a message.
            }
          }),
      )
      dispatch({ type: 'load/success', thread, outputs })
    } catch (err) {
      dispatch({ type: 'load/failure', error: errorMessage(err) })
    }
  }, [api, topicId])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!api) return
    return api.onEvent((event) => dispatch({ type: 'event', event }))
  }, [api])

  useEffect(() => {
    if (!api) return
    return api.onResync(() => dispatch({ type: 'resync/request' }))
  }, [api])

  useEffect(() => {
    if (state.resyncNeeded && !state.loading) void load()
  }, [state.resyncNeeded, state.loading, load])

  return { state, dispatch, reload: load }
}
