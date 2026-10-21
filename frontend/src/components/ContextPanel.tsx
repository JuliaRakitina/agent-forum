import { useEffect, useState, type FormEvent } from 'react'
import type { ContextDoc, ContextRevision } from '../api/types'
import { errorMessage, formatTime } from '../lib/time'

interface Props {
  topicId: string | null
  context: ContextRevision | null
  onSave: (doc: ContextDoc) => Promise<unknown>
}

interface Draft {
  goal: string
  constraints: string
  decisions: string
  done: string
  current: string
  openQuestions: string
}

const SEPARATOR = ' — '

function toDraft(doc: ContextDoc | undefined): Draft {
  return {
    goal: doc?.goal ?? '',
    constraints: (doc?.constraints ?? []).join('\n'),
    decisions: (doc?.decisions ?? []).map((d) => (d.reason ? `${d.text}${SEPARATOR}${d.reason}` : d.text)).join('\n'),
    done: (doc?.done ?? []).join('\n'),
    current: doc?.current ?? '',
    openQuestions: (doc?.openQuestions ?? []).join('\n'),
  }
}

function lines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l.length > 0)
}

export function toDoc(draft: Draft, previous: ContextDoc | undefined): ContextDoc {
  return {
    goal: draft.goal.trim(),
    constraints: lines(draft.constraints),
    decisions: lines(draft.decisions).map((line) => {
      const idx = line.indexOf(SEPARATOR)
      if (idx < 0) return { text: line }
      return { text: line.slice(0, idx).trim(), reason: line.slice(idx + SEPARATOR.length).trim() }
    }),
    done: lines(draft.done),
    current: draft.current.trim(),
    openQuestions: lines(draft.openQuestions),
    references: previous?.references ?? [],
  }
}

export function ContextPanel({ topicId, context, onSave }: Props) {
  const [draft, setDraft] = useState<Draft>(() => toDraft(context?.doc))
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setDraft(toDraft(context?.doc))
    setDirty(false)
    setError(null)
  }, [context?.id, topicId])

  function update<K extends keyof Draft>(key: K, value: string) {
    setDraft((d) => ({ ...d, [key]: value }))
    setDirty(true)
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!topicId) return
    setBusy(true)
    setError(null)
    try {
      await onSave(toDoc(draft, context?.doc))
      setDirty(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (!topicId) return <p className="muted">Select a topic to see its shared context.</p>

  return (
    <form className="panel-form" onSubmit={submit}>
      <div className="panel-meta">
        {context ? (
          <span>
            Revision {context.version} · {context.authorKind} · {formatTime(context.createdAt)}
          </span>
        ) : (
          <span className="muted">No context yet. Fill in what matters and save the first revision.</span>
        )}
      </div>
      <label>
        Goal
        <textarea rows={2} value={draft.goal} onChange={(e) => update('goal', e.target.value)} />
      </label>
      <label>
        Constraints <span className="muted small">(one per line)</span>
        <textarea rows={3} value={draft.constraints} onChange={(e) => update('constraints', e.target.value)} />
      </label>
      <label>
        Decisions <span className="muted small">(one per line: decision — reason)</span>
        <textarea rows={3} value={draft.decisions} onChange={(e) => update('decisions', e.target.value)} />
      </label>
      <label>
        Done <span className="muted small">(one per line)</span>
        <textarea rows={2} value={draft.done} onChange={(e) => update('done', e.target.value)} />
      </label>
      <label>
        Where we stopped
        <textarea rows={2} value={draft.current} onChange={(e) => update('current', e.target.value)} />
      </label>
      <label>
        Open questions <span className="muted small">(one per line)</span>
        <textarea rows={3} value={draft.openQuestions} onChange={(e) => update('openQuestions', e.target.value)} />
      </label>
      {error && <p className="form-error">{error}</p>}
      <div className="panel-actions">
        <button type="submit" className="btn btn-primary" disabled={busy || !dirty}>
          {busy ? 'Saving…' : context ? 'Save as new revision' : 'Save context'}
        </button>
        {dirty && (
          <button type="button" className="btn btn-ghost" onClick={() => { setDraft(toDraft(context?.doc)); setDirty(false) }}>
            Discard
          </button>
        )}
      </div>
    </form>
  )
}
