import { useState, type KeyboardEvent } from 'react'
import type { AskMode, Participant, PostInput } from '../api/types'
import { ASK_MODES } from '../lib/labels'
import { mentionable } from '../lib/participants'
import { errorMessage } from '../lib/time'

interface Props {
  topicId: string | null
  participants: Participant[]
  onSend: (input: PostInput) => Promise<unknown>
}

export function Composer({ topicId, participants, onSend }: Props) {
  const [body, setBody] = useState('')
  const [mentions, setMentions] = useState<string[]>([])
  const [mode, setMode] = useState<AskMode>('one')
  const [autoReveal, setAutoReveal] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const agents = mentionable(participants)
  const canSend = topicId !== null && body.trim().length > 0 && !sending

  function toggle(id: string) {
    setMentions((m) => (m.includes(id) ? m.filter((x) => x !== id) : [...m, id]))
  }

  async function send() {
    if (!canSend || !topicId) return
    setSending(true)
    setError(null)
    try {
      await onSend({ topicId, body: body.trim(), mentions, mode, autoReveal })
      setBody('')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSending(false)
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      void send()
    }
  }

  const activeMode = ASK_MODES.find((m) => m.value === mode)

  return (
    <form
      className="composer"
      onSubmit={(e) => {
        e.preventDefault()
        void send()
      }}
    >
      <textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={topicId ? 'Write a message. Mention participants below to ask them. ⌘/Ctrl+Enter sends.' : 'Select a topic first.'}
        disabled={topicId === null || sending}
        rows={3}
        aria-label="Message"
      />
      <div className="composer-row">
        <div className="chips" role="group" aria-label="Mentions">
          {agents.length === 0 && <span className="muted small">No agent participants configured.</span>}
          {agents.map((p) => (
            <button
              key={p.id}
              type="button"
              className={`chip chip-toggle ${mentions.includes(p.id) ? 'is-on' : ''}`}
              onClick={() => toggle(p.id)}
              disabled={!p.enabled}
              aria-pressed={mentions.includes(p.id)}
              title={p.enabled ? `Mention ${p.name}` : `${p.name} is disabled`}
            >
              @{p.name}
            </button>
          ))}
        </div>
        <div className="segmented" role="radiogroup" aria-label="Answer mode">
          {ASK_MODES.map((m) => (
            <button
              key={m.value}
              type="button"
              role="radio"
              aria-checked={mode === m.value}
              className={mode === m.value ? 'is-on' : ''}
              onClick={() => setMode(m.value)}
              title={m.hint}
            >
              {m.label}
            </button>
          ))}
        </div>
        {mode === 'independent' && (
          <label className="checkbox">
            <input type="checkbox" checked={autoReveal} onChange={(e) => setAutoReveal(e.target.checked)} />
            Reveal automatically
          </label>
        )}
        <button type="submit" className="btn btn-primary" disabled={!canSend}>
          {sending ? 'Sending…' : 'Send'}
        </button>
      </div>
      <div className="composer-hint">
        {error ? <span className="form-error">{error}</span> : <span className="muted small">{activeMode?.hint}</span>}
      </div>
    </form>
  )
}
