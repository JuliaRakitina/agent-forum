import { useState, type FormEvent } from 'react'
import type { Participant, Topic, TopicKind, Workspace } from '../api/types'
import { mentionable } from '../lib/participants'
import { errorMessage, relativeTime } from '../lib/time'

interface Props {
  workspace: Workspace
  topics: Topic[]
  selectedTopicId: string | null
  participants: Participant[]
  onSelectTopic: (id: string) => void
  onCreateTopic: (title: string, kind: TopicKind) => Promise<unknown>
}

const KINDS: { value: TopicKind; label: string }[] = [
  { value: 'discussion', label: 'Discussion' },
  { value: 'question', label: 'Question' },
  { value: 'task', label: 'Task' },
]

export function Sidebar({ workspace, topics, selectedTopicId, participants, onSelectTopic, onCreateTopic }: Props) {
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const [kind, setKind] = useState<TopicKind>('discussion')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!title.trim()) return
    setBusy(true)
    setError(null)
    try {
      await onCreateTopic(title, kind)
      setTitle('')
      setCreating(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <aside className="sidebar">
      <div className="sidebar-section">
        <div className="sidebar-heading">
          <h2>Topics</h2>
          <button type="button" className="btn btn-small" onClick={() => setCreating((v) => !v)} aria-expanded={creating}>
            {creating ? 'Close' : 'New topic'}
          </button>
        </div>
        {creating && (
          <form className="new-topic" onSubmit={submit}>
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Topic title"
              aria-label="Topic title"
              maxLength={200}
            />
            <div className="new-topic-row">
              <select value={kind} onChange={(e) => setKind(e.target.value as TopicKind)} aria-label="Topic kind">
                {KINDS.map((k) => (
                  <option key={k.value} value={k.value}>
                    {k.label}
                  </option>
                ))}
              </select>
              <button type="submit" className="btn btn-primary btn-small" disabled={busy || !title.trim()}>
                Create
              </button>
            </div>
            {error && <p className="form-error">{error}</p>}
          </form>
        )}
        <ul className="topic-list">
          {topics.length === 0 && <li className="muted">No topics yet.</li>}
          {topics.map((t) => (
            <li key={t.id}>
              <button
                type="button"
                className={`topic-item ${t.id === selectedTopicId ? 'is-selected' : ''}`}
                onClick={() => onSelectTopic(t.id)}
                aria-current={t.id === selectedTopicId ? 'true' : undefined}
              >
                <span className="topic-title">{t.title}</span>
                <span className="topic-meta">
                  <span className={`badge badge-${t.kind}`}>{t.kind}</span>
                  <span className="muted">{relativeTime(t.updatedAt)}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      </div>
      <div className="sidebar-section">
        <div className="sidebar-heading">
          <h2>Participants</h2>
        </div>
        <ul className="participant-list">
          <li className="participant-row">
            <span className="dot dot-on" aria-hidden="true" />
            <span className="participant-name">You</span>
            <span className="muted">human</span>
          </li>
          {mentionable(participants).map((p) => (
            <li key={p.id} className="participant-row">
              <span className={`dot ${p.enabled ? 'dot-on' : 'dot-off'}`} aria-hidden="true" title={p.enabled ? 'enabled' : 'disabled'} />
              <span className="participant-name">{p.name}</span>
              <span className="muted">
                {p.provider}
                {p.model ? ` · ${p.model}` : ''}
              </span>
            </li>
          ))}
        </ul>
        <p className="muted small">Workspace root: {workspace.rootDir || 'not set'}</p>
      </div>
    </aside>
  )
}
