import { useEffect, useState, type FormEvent } from 'react'
import type { Capabilities, Participant, ParticipationMode, Provider, RunProfile, ToolPolicy, Workspace } from '../api/types'
import { mentionable } from '../lib/participants'
import { errorMessage } from '../lib/time'

interface Props {
  workspace: Workspace
  participants: Participant[]
  capabilities: Capabilities[]
  onSave: (p: Participant) => Promise<unknown>
}

const MODES: { value: ParticipationMode; label: string }[] = [
  { value: 'mentioned', label: 'When mentioned' },
  { value: 'always', label: 'Always' },
  { value: 'manual', label: 'Manual only' },
]

const TOOLS: { value: ToolPolicy; label: string }[] = [
  { value: 'none', label: 'No tools (discussion)' },
  { value: 'read_only', label: 'Read files in the run directory' },
  { value: 'workspace_write', label: 'Edit files in the run directory' },
]

function blank(workspaceId: string, provider: Provider): Participant {
  return { id: '', workspaceId, name: '', provider, model: '', role: '', effort: '', maxWords: 150, mode: 'mentioned', enabled: true, profile: 'subscription', tools: 'none' }
}

export function ParticipantsPanel({ workspace, participants, capabilities, onSave }: Props) {
  const agents = mentionable(participants)
  const [selectedId, setSelectedId] = useState<string | null>(agents[0]?.id ?? null)
  const [draft, setDraft] = useState<Participant | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (selectedId === null) return
    const current = participants.find((p) => p.id === selectedId)
    if (current) setDraft({ ...current })
  }, [selectedId, participants])

  const caps = draft ? capabilities.find((c) => c.provider === draft.provider) : undefined
  const providers = capabilities.map((c) => c.provider)

  function startNew() {
    const provider = providers[0] ?? 'claude'
    setSelectedId(null)
    setDraft(blank(workspace.id, provider))
    setError(null)
  }

  function set<K extends keyof Participant>(key: K, value: Participant[K]) {
    setDraft((d) => (d ? { ...d, [key]: value } : d))
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!draft) return
    setBusy(true)
    setError(null)
    try {
      const saved = (await onSave(draft)) as Participant | unknown
      if (saved && typeof saved === 'object' && 'id' in saved && typeof saved.id === 'string') setSelectedId(saved.id)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="participants-panel">
      <div className="panel-meta">
        <span className="muted small">Agents of {workspace.name}</span>
        <button type="button" className="btn btn-small" onClick={startNew}>
          Add participant
        </button>
      </div>
      <ul className="participant-picker" role="listbox" aria-label="Participants">
        {agents.map((p) => (
          <li key={p.id}>
            <button
              type="button"
              role="option"
              aria-selected={p.id === selectedId}
              className={`participant-pick ${p.id === selectedId ? 'is-selected' : ''}`}
              onClick={() => setSelectedId(p.id)}
            >
              <span className={`dot ${p.enabled ? 'dot-on' : 'dot-off'}`} aria-hidden="true" />
              {p.name} <span className="muted">· {p.provider}</span>
            </button>
          </li>
        ))}
      </ul>
      {draft && (
        <form className="panel-form" onSubmit={submit}>
          <label>
            Name
            <input value={draft.name} onChange={(e) => set('name', e.target.value)} required maxLength={60} />
          </label>
          <label>
            Provider
            <select
              value={draft.provider}
              onChange={(e) => {
                const provider = e.target.value as Provider
                const next = capabilities.find((c) => c.provider === provider)
                setDraft((d) =>
                  d
                    ? {
                        ...d,
                        provider,
                        effort: next?.efforts?.includes(d.effort ?? '') ? d.effort : '',
                        profile: next?.profiles.includes(d.profile) ? d.profile : (next?.profiles[0] ?? 'subscription'),
                      }
                    : d,
                )
              }}
            >
              {providers.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </label>
          <label>
            Model <span className="muted small">(empty = CLI default)</span>
            <input value={draft.model ?? ''} onChange={(e) => set('model', e.target.value)} placeholder="e.g. sonnet" />
          </label>
          <label>
            Role
            <textarea rows={3} value={draft.role ?? ''} onChange={(e) => set('role', e.target.value)} placeholder="Instructions prepended to every prompt" />
          </label>
          <div className="field-row">
            <label>
              Effort
              <select value={draft.effort ?? ''} onChange={(e) => set('effort', e.target.value)} disabled={!caps?.efforts?.length}>
                <option value="">default</option>
                {(caps?.efforts ?? []).map((lvl) => (
                  <option key={lvl} value={lvl}>
                    {lvl}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Max words
              <input type="number" min={0} max={5000} value={draft.maxWords ?? 0} onChange={(e) => set('maxWords', Number(e.target.value))} />
            </label>
          </div>
          <label>
            Participation
            <select value={draft.mode} onChange={(e) => set('mode', e.target.value as ParticipationMode)}>
              {MODES.map((m) => (
                <option key={m.value} value={m.value}>
                  {m.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            Profile
            <select value={draft.profile} onChange={(e) => set('profile', e.target.value as RunProfile)}>
              {(caps?.profiles ?? ['subscription']).map((p) => (
                <option key={p} value={p}>
                  {p === 'subscription' ? 'CLI login (subscription)' : 'API key from environment'}
                </option>
              ))}
            </select>
          </label>
          <label>
            Tools
            <select value={draft.tools} onChange={(e) => set('tools', e.target.value as ToolPolicy)}>
              {TOOLS.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </label>
          <label className="checkbox">
            <input type="checkbox" checked={draft.enabled} onChange={(e) => set('enabled', e.target.checked)} />
            Enabled
          </label>
          {caps && (
            <dl className="caps">
              <dt>Streaming</dt>
              <dd>{caps.streaming ? 'yes' : 'no'}</dd>
              <dt>Cancel</dt>
              <dd>{caps.cancel ? 'yes' : 'no'}</dd>
              <dt>Tool approvals</dt>
              <dd>{caps.toolApprovals ? 'yes' : 'not yet'}</dd>
              <dt>Max isolation</dt>
              <dd>{caps.maxIsolation}</dd>
            </dl>
          )}
          {error && <p className="form-error">{error}</p>}
          <div className="panel-actions">
            <button type="submit" className="btn btn-primary" disabled={busy || !draft.name.trim()}>
              {busy ? 'Saving…' : draft.id ? 'Save' : 'Create'}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
