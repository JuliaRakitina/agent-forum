import type { ApiMode, ExportFormat } from '../api/types'

interface Props {
  workspaceName: string
  topicTitle: string | null
  mode: ApiMode
  /** Present when the backend can export and a topic is open. */
  onExport?: (format: ExportFormat) => void
}

export function TopBar({ workspaceName, topicTitle, mode, onExport }: Props) {
  return (
    <header className="topbar">
      <div className="topbar-brand">
        <span className="topbar-app">Agent Forum</span>
        <span className="topbar-sep" aria-hidden="true">
          /
        </span>
        <span className="topbar-workspace">{workspaceName}</span>
      </div>
      <h1 className="topbar-title">{topicTitle ?? 'No topic selected'}</h1>
      {onExport && (
        <div className="topbar-actions" role="group" aria-label="Export topic">
          <button
            type="button"
            className="btn btn-small btn-ghost"
            title="Export the visible discussion as Markdown; sealed answers are never included"
            onClick={() => onExport('markdown')}
          >
            Export .md
          </button>
          <button
            type="button"
            className="btn btn-small btn-ghost"
            title="Export the visible discussion as JSON; sealed answers are never included"
            onClick={() => onExport('json')}
          >
            .json
          </button>
        </div>
      )}
      <div className="topbar-status" title={mode === 'wails' ? 'Connected to the Go core' : 'In-memory mock backend'}>
        <span className={`dot ${mode === 'wails' ? 'dot-on' : 'dot-mock'}`} aria-hidden="true" />
        {mode === 'wails' ? 'Wails' : 'Mock'}
      </div>
    </header>
  )
}
