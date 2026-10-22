package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
)

// --- workspaces -----------------------------------------------------------

// CreateWorkspace inserts a workspace.
func (t *Tx) CreateWorkspace(w domain.Workspace) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO workspaces (id, name, root_dir, created_at) VALUES (?, ?, ?, ?)`,
		w.ID, w.Name, w.RootDir, formatTime(w.CreatedAt))
	return wrap("create workspace", err)
}

// GetWorkspace loads a workspace by ID.
func (t *Tx) GetWorkspace(id domain.ID) (domain.Workspace, error) {
	var w domain.Workspace
	var created string
	err := t.tx.QueryRowContext(t.ctx, `SELECT id, name, root_dir, created_at FROM workspaces WHERE id = ?`, id).
		Scan(&w.ID, &w.Name, &w.RootDir, &created)
	if err != nil {
		return w, notFound("workspace", err)
	}
	w.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	return w, nil
}

// ListWorkspaces returns every workspace, oldest first.
func (t *Tx) ListWorkspaces() ([]domain.Workspace, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT id, name, root_dir, created_at FROM workspaces ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Workspace
	for rows.Next() {
		var w domain.Workspace
		var created string
		if err := rows.Scan(&w.ID, &w.Name, &w.RootDir, &created); err != nil {
			return nil, err
		}
		w.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- topics ---------------------------------------------------------------

// CreateTopic inserts a topic.
func (t *Tx) CreateTopic(tp domain.Topic) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO topics (id, workspace_id, title, kind, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.WorkspaceID, tp.Title, tp.Kind, formatTime(tp.CreatedAt), formatTime(tp.UpdatedAt))
	return wrap("create topic", err)
}

const topicColumns = `id, workspace_id, title, kind, created_at, updated_at`

func scanTopic(s interface{ Scan(...any) error }) (domain.Topic, error) {
	var tp domain.Topic
	var created, updated string
	if err := s.Scan(&tp.ID, &tp.WorkspaceID, &tp.Title, &tp.Kind, &created, &updated); err != nil {
		return tp, err
	}
	tp.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	tp.UpdatedAt = parseTime(sql.NullString{String: updated, Valid: true})
	return tp, nil
}

// GetTopic loads a topic by ID.
func (t *Tx) GetTopic(id domain.ID) (domain.Topic, error) {
	tp, err := scanTopic(t.tx.QueryRowContext(t.ctx, `SELECT `+topicColumns+` FROM topics WHERE id = ?`, id))
	if err != nil {
		return tp, notFound("topic", err)
	}
	return tp, nil
}

// ListTopics returns the topics of a workspace, most recently updated first.
func (t *Tx) ListTopics(workspaceID domain.ID) ([]domain.Topic, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+topicColumns+` FROM topics WHERE workspace_id = ? ORDER BY updated_at DESC, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Topic
	for rows.Next() {
		tp, err := scanTopic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tp)
	}
	return out, rows.Err()
}

// TouchTopic records activity on a topic.
func (t *Tx) TouchTopic(id domain.ID, at time.Time) error {
	_, err := t.tx.ExecContext(t.ctx, `UPDATE topics SET updated_at = ? WHERE id = ?`, formatTime(at), id)
	return wrap("touch topic", err)
}

// --- participants ---------------------------------------------------------

// PutParticipant inserts or replaces a participant.
func (t *Tx) PutParticipant(p domain.Participant) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO participants
		(id, workspace_id, name, provider, model, role, effort, max_words, mode, enabled, profile, tools)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, provider = excluded.provider, model = excluded.model,
		role = excluded.role, effort = excluded.effort, max_words = excluded.max_words, mode = excluded.mode,
		enabled = excluded.enabled, profile = excluded.profile, tools = excluded.tools`,
		p.ID, p.WorkspaceID, p.Name, p.Provider, p.Model, p.Role, p.Effort, p.MaxWords, p.Mode, boolInt(p.Enabled), p.Profile, p.Tools)
	return wrap("put participant", err)
}

const participantColumns = `id, workspace_id, name, provider, model, role, effort, max_words, mode, enabled, profile, tools`

func scanParticipant(s interface{ Scan(...any) error }) (domain.Participant, error) {
	var p domain.Participant
	var enabled int
	err := s.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Provider, &p.Model, &p.Role, &p.Effort, &p.MaxWords, &p.Mode, &enabled, &p.Profile, &p.Tools)
	p.Enabled = enabled != 0
	return p, err
}

// GetParticipant loads a participant by ID.
func (t *Tx) GetParticipant(id domain.ID) (domain.Participant, error) {
	p, err := scanParticipant(t.tx.QueryRowContext(t.ctx, `SELECT `+participantColumns+` FROM participants WHERE id = ?`, id))
	if err != nil {
		return p, notFound("participant", err)
	}
	return p, nil
}

// ListParticipants returns the participants of a workspace by name.
func (t *Tx) ListParticipants(workspaceID domain.ID) ([]domain.Participant, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+participantColumns+` FROM participants WHERE workspace_id = ? ORDER BY name, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Participant
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- messages -------------------------------------------------------------

// CreateMessage inserts a message.
func (t *Tx) CreateMessage(m domain.Message) error {
	mentions, err := json.Marshal(nonNil(m.Mentions))
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO messages
		(id, topic_id, parent_id, author_kind, participant_id, body, mentions, run_id, round_id, visibility, corrects_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.TopicID, nullString(string(m.ParentID)), m.AuthorKind, nullString(string(m.ParticipantID)), m.Body, string(mentions),
		nullString(string(m.RunID)), nullString(string(m.RoundID)), m.Visibility, nullString(string(m.CorrectsID)), formatTime(m.CreatedAt))
	return wrap("create message", err)
}

const messageColumns = `id, topic_id, parent_id, author_kind, participant_id, body, mentions, run_id, round_id, visibility, corrects_id, created_at`

func scanMessage(s interface{ Scan(...any) error }) (domain.Message, error) {
	var m domain.Message
	var parent, participant, run, round, corrects sql.NullString
	var mentions, created string
	if err := s.Scan(&m.ID, &m.TopicID, &parent, &m.AuthorKind, &participant, &m.Body, &mentions, &run, &round, &m.Visibility, &corrects, &created); err != nil {
		return m, err
	}
	m.ParentID, m.ParticipantID, m.RunID, m.RoundID, m.CorrectsID = domain.ID(parent.String), domain.ID(participant.String), domain.ID(run.String), domain.ID(round.String), domain.ID(corrects.String)
	m.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	if err := json.Unmarshal([]byte(mentions), &m.Mentions); err != nil {
		return m, err
	}
	return m, nil
}

// GetMessage loads a message by ID.
func (t *Tx) GetMessage(id domain.ID) (domain.Message, error) {
	m, err := scanMessage(t.tx.QueryRowContext(t.ctx, `SELECT `+messageColumns+` FROM messages WHERE id = ?`, id))
	if err != nil {
		return m, notFound("message", err)
	}
	return m, nil
}

// MessageQuery selects messages of a topic.
type MessageQuery struct {
	TopicID domain.ID
	// IncludeSealed returns sealed messages too. Only the orchestrator
	// should set it; the UI and context assembly never see sealed bodies.
	IncludeSealed bool
	// Before returns messages created before this time (exclusive), for
	// paging backwards. Zero means no bound.
	Before time.Time
	// Limit bounds the number of rows; zero means no limit.
	Limit int
}

// ListMessages returns messages in chronological order.
func (t *Tx) ListMessages(q MessageQuery) ([]domain.Message, error) {
	var where []string
	var args []any
	where = append(where, "topic_id = ?")
	args = append(args, q.TopicID)
	if !q.IncludeSealed {
		where = append(where, "visibility = ?")
		args = append(args, domain.VisibilityVisible)
	}
	if !q.Before.IsZero() {
		where = append(where, "created_at < ?")
		args = append(args, formatTime(q.Before))
	}
	query := `SELECT ` + messageColumns + ` FROM messages WHERE ` + strings.Join(where, " AND ")
	if q.Limit > 0 {
		// Take the newest Limit rows, then flip to chronological order.
		query = `SELECT * FROM (` + query + ` ORDER BY created_at DESC, id DESC LIMIT ?) ORDER BY created_at, id`
		args = append(args, q.Limit)
	} else {
		query += ` ORDER BY created_at, id`
	}
	rows, err := t.tx.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RevealRoundMessages makes every sealed message of a round visible and
// returns the messages that were sealed before the call.
func (t *Tx) RevealRoundMessages(roundID domain.ID) ([]domain.Message, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+messageColumns+` FROM messages WHERE round_id = ? AND visibility = ? ORDER BY created_at, id`,
		roundID, domain.VisibilitySealed)
	if err != nil {
		return nil, err
	}
	var sealed []domain.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		sealed = append(sealed, m)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := t.tx.ExecContext(t.ctx, `UPDATE messages SET visibility = ? WHERE round_id = ? AND visibility = ?`,
		domain.VisibilityVisible, roundID, domain.VisibilitySealed); err != nil {
		return nil, wrap("reveal messages", err)
	}
	for i := range sealed {
		sealed[i].Visibility = domain.VisibilityVisible
	}
	return sealed, nil
}

// --- rounds ---------------------------------------------------------------

// CreateRound inserts a round.
func (t *Tx) CreateRound(r domain.Round) error {
	ids, err := json.Marshal(nonNil(r.ParticipantIDs))
	if err != nil {
		return err
	}
	inputs, err := json.Marshal(r.Inputs)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO rounds
		(id, topic_id, kind, status, prompt, participant_ids, inputs, auto_reveal, created_at, revealed_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TopicID, r.Kind, r.Status, r.Prompt, string(ids), string(inputs), boolInt(r.AutoReveal), formatTime(r.CreatedAt), nullTime(r.RevealedAt), nullTime(r.FinishedAt))
	return wrap("create round", err)
}

const roundColumns = `id, topic_id, kind, status, prompt, participant_ids, inputs, auto_reveal, created_at, revealed_at, finished_at`

func scanRound(s interface{ Scan(...any) error }) (domain.Round, error) {
	var r domain.Round
	var ids, inputs, created string
	var autoReveal int
	var revealed, finished sql.NullString
	if err := s.Scan(&r.ID, &r.TopicID, &r.Kind, &r.Status, &r.Prompt, &ids, &inputs, &autoReveal, &created, &revealed, &finished); err != nil {
		return r, err
	}
	r.AutoReveal = autoReveal != 0
	r.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	r.RevealedAt, r.FinishedAt = parseTime(revealed), parseTime(finished)
	if err := json.Unmarshal([]byte(ids), &r.ParticipantIDs); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(inputs), &r.Inputs); err != nil {
		return r, err
	}
	return r, nil
}

// GetRound loads a round by ID.
func (t *Tx) GetRound(id domain.ID) (domain.Round, error) {
	r, err := scanRound(t.tx.QueryRowContext(t.ctx, `SELECT `+roundColumns+` FROM rounds WHERE id = ?`, id))
	if err != nil {
		return r, notFound("round", err)
	}
	return r, nil
}

// UpdateRound stores status and timestamps of a round.
func (t *Tx) UpdateRound(r domain.Round) error {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE rounds SET status = ?, revealed_at = ?, finished_at = ? WHERE id = ?`,
		r.Status, nullTime(r.RevealedAt), nullTime(r.FinishedAt), r.ID)
	if err != nil {
		return wrap("update round", err)
	}
	return affected("round", res)
}

// ListRoundsByStatus returns rounds in any of the given states.
func (t *Tx) ListRoundsByStatus(statuses ...domain.RoundStatus) ([]domain.Round, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	args := make([]any, len(statuses))
	for i, s := range statuses {
		args[i] = s
	}
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+roundColumns+` FROM rounds WHERE status IN (`+placeholders(len(statuses))+`) ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Round
	for rows.Next() {
		r, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRoundsByTopic returns the rounds of a topic, oldest first.
func (t *Tx) ListRoundsByTopic(topicID domain.ID) ([]domain.Round, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+roundColumns+` FROM rounds WHERE topic_id = ? ORDER BY created_at, id`, topicID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Round
	for rows.Next() {
		r, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- runs -----------------------------------------------------------------

// CreateRun inserts a run.
func (t *Tx) CreateRun(r domain.Run) error {
	usage, err := json.Marshal(r.Usage)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO runs
		(id, topic_id, participant_id, round_id, trigger_message_id, status, profile, tools, isolation, work_dir, native_session_id, result_message_id, error, usage, created_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TopicID, r.ParticipantID, nullString(string(r.RoundID)), nullString(string(r.TriggerMessageID)), r.Status, r.Profile, r.Tools, r.Isolation,
		r.WorkDir, r.NativeSessionID, nullString(string(r.ResultMessageID)), r.Error, string(usage), formatTime(r.CreatedAt), nullTime(r.StartedAt), nullTime(r.FinishedAt))
	return wrap("create run", err)
}

const runColumns = `id, topic_id, participant_id, round_id, trigger_message_id, status, profile, tools, isolation, work_dir, native_session_id, result_message_id, error, usage, created_at, started_at, finished_at`

func scanRun(s interface{ Scan(...any) error }) (domain.Run, error) {
	var r domain.Run
	var round, trigger, result, started, finished sql.NullString
	var usage, created string
	if err := s.Scan(&r.ID, &r.TopicID, &r.ParticipantID, &round, &trigger, &r.Status, &r.Profile, &r.Tools, &r.Isolation, &r.WorkDir, &r.NativeSessionID, &result, &r.Error, &usage, &created, &started, &finished); err != nil {
		return r, err
	}
	r.RoundID, r.TriggerMessageID, r.ResultMessageID = domain.ID(round.String), domain.ID(trigger.String), domain.ID(result.String)
	r.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	r.StartedAt, r.FinishedAt = parseTime(started), parseTime(finished)
	if err := json.Unmarshal([]byte(usage), &r.Usage); err != nil {
		return r, err
	}
	return r, nil
}

// GetRun loads a run by ID.
func (t *Tx) GetRun(id domain.ID) (domain.Run, error) {
	r, err := scanRun(t.tx.QueryRowContext(t.ctx, `SELECT `+runColumns+` FROM runs WHERE id = ?`, id))
	if err != nil {
		return r, notFound("run", err)
	}
	return r, nil
}

// UpdateRun stores the mutable fields of a run.
func (t *Tx) UpdateRun(r domain.Run) error {
	usage, err := json.Marshal(r.Usage)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE runs SET status = ?, work_dir = ?, native_session_id = ?, result_message_id = ?, error = ?, usage = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		r.Status, r.WorkDir, r.NativeSessionID, nullString(string(r.ResultMessageID)), r.Error, string(usage), nullTime(r.StartedAt), nullTime(r.FinishedAt), r.ID)
	if err != nil {
		return wrap("update run", err)
	}
	return affected("run", res)
}

// ListRunsByRound returns the runs of a round in creation order.
func (t *Tx) ListRunsByRound(roundID domain.ID) ([]domain.Run, error) {
	return t.listRuns(`round_id = ?`, roundID)
}

// ListRunsByTopic returns the runs of a topic in creation order.
func (t *Tx) ListRunsByTopic(topicID domain.ID) ([]domain.Run, error) {
	return t.listRuns(`topic_id = ?`, topicID)
}

// ListRunsByStatus returns runs in any of the given states.
func (t *Tx) ListRunsByStatus(statuses ...domain.RunStatus) ([]domain.Run, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	args := make([]any, len(statuses))
	for i, s := range statuses {
		args[i] = s
	}
	return t.listRuns(`status IN (`+placeholders(len(statuses))+`)`, args...)
}

func (t *Tx) listRuns(where string, args ...any) ([]domain.Run, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+runColumns+` FROM runs WHERE `+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AppendRunOutput stores one streamed chunk of a run.
func (t *Tx) AppendRunOutput(runID domain.ID, index int, text string) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO run_output (run_id, idx, text) VALUES (?, ?, ?)`, runID, index, text)
	return wrap("append run output", err)
}

// RunOutput returns the streamed chunks of a run joined in order.
func (t *Tx) RunOutput(runID domain.ID) (string, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT text FROM run_output WHERE run_id = ? ORDER BY idx`, runID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), rows.Err()
}

// --- context revisions ----------------------------------------------------

// CreateContextRevision inserts a revision, assigning the next version
// number of the topic when rev.Version is zero.
func (t *Tx) CreateContextRevision(rev *domain.ContextRevision) error {
	if rev.Version == 0 {
		var maxVersion sql.NullInt64
		if err := t.tx.QueryRowContext(t.ctx, `SELECT MAX(version) FROM context_revisions WHERE topic_id = ?`, rev.TopicID).Scan(&maxVersion); err != nil {
			return err
		}
		rev.Version = int(maxVersion.Int64) + 1
	}
	doc, err := json.Marshal(rev.Doc)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO context_revisions (id, topic_id, version, status, author_kind, participant_id, run_id, doc, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rev.ID, rev.TopicID, rev.Version, rev.Status, rev.AuthorKind, nullString(string(rev.ParticipantID)), nullString(string(rev.RunID)), string(doc), formatTime(rev.CreatedAt))
	return wrap("create context revision", err)
}

const revisionColumns = `id, topic_id, version, status, author_kind, participant_id, run_id, doc, created_at`

func scanRevision(s interface{ Scan(...any) error }) (domain.ContextRevision, error) {
	var r domain.ContextRevision
	var participant, run sql.NullString
	var doc, created string
	if err := s.Scan(&r.ID, &r.TopicID, &r.Version, &r.Status, &r.AuthorKind, &participant, &run, &doc, &created); err != nil {
		return r, err
	}
	r.ParticipantID, r.RunID = domain.ID(participant.String), domain.ID(run.String)
	r.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
	if err := json.Unmarshal([]byte(doc), &r.Doc); err != nil {
		return r, err
	}
	return r, nil
}

// GetContextRevision loads a revision by ID.
func (t *Tx) GetContextRevision(id domain.ID) (domain.ContextRevision, error) {
	r, err := scanRevision(t.tx.QueryRowContext(t.ctx, `SELECT `+revisionColumns+` FROM context_revisions WHERE id = ?`, id))
	if err != nil {
		return r, notFound("context revision", err)
	}
	return r, nil
}

// CurrentContext returns the newest accepted revision of a topic, or
// ErrNotFound when the topic has none.
func (t *Tx) CurrentContext(topicID domain.ID) (domain.ContextRevision, error) {
	r, err := scanRevision(t.tx.QueryRowContext(t.ctx, `SELECT `+revisionColumns+` FROM context_revisions WHERE topic_id = ? AND status = ? ORDER BY version DESC LIMIT 1`,
		topicID, domain.RevisionAccepted))
	if err != nil {
		return r, notFound("context revision", err)
	}
	return r, nil
}

// ListContextRevisions returns every revision of a topic, oldest first.
func (t *Tx) ListContextRevisions(topicID domain.ID) ([]domain.ContextRevision, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT `+revisionColumns+` FROM context_revisions WHERE topic_id = ? ORDER BY version`, topicID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.ContextRevision
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRevisionStatus accepts or discards a revision.
func (t *Tx) SetRevisionStatus(id domain.ID, status domain.RevisionStatus) error {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE context_revisions SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return wrap("set revision status", err)
	}
	return affected("context revision", res)
}

// --- artifacts ------------------------------------------------------------

// CreateArtifactVersion inserts an artifact version, assigning the next
// version number for the name when av.Version is zero.
func (t *Tx) CreateArtifactVersion(av *domain.ArtifactVersion) error {
	if av.Version == 0 {
		var maxVersion sql.NullInt64
		if err := t.tx.QueryRowContext(t.ctx, `SELECT MAX(version) FROM artifact_versions WHERE topic_id = ? AND name = ?`, av.TopicID, av.Name).Scan(&maxVersion); err != nil {
			return err
		}
		av.Version = int(maxVersion.Int64) + 1
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO artifact_versions (id, topic_id, name, version, sha256, size, path, run_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		av.ID, av.TopicID, av.Name, av.Version, av.SHA256, av.Size, av.Path, nullString(string(av.RunID)), formatTime(av.CreatedAt))
	return wrap("create artifact version", err)
}

// ListArtifactVersions returns the artifact versions of a topic.
func (t *Tx) ListArtifactVersions(topicID domain.ID) ([]domain.ArtifactVersion, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT id, topic_id, name, version, sha256, size, path, run_id, created_at FROM artifact_versions WHERE topic_id = ? ORDER BY name, version`, topicID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.ArtifactVersion
	for rows.Next() {
		var av domain.ArtifactVersion
		var run sql.NullString
		var created string
		if err := rows.Scan(&av.ID, &av.TopicID, &av.Name, &av.Version, &av.SHA256, &av.Size, &av.Path, &run, &created); err != nil {
			return nil, err
		}
		av.RunID = domain.ID(run.String)
		av.CreatedAt = parseTime(sql.NullString{String: created, Valid: true})
		out = append(out, av)
	}
	return out, rows.Err()
}

// --- events ---------------------------------------------------------------

// AppendEvent stores an event and fills in its sequence number.
func (t *Tx) AppendEvent(ev *domain.Event) error {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO events (type, topic_id, at, payload) VALUES (?, ?, ?, ?)`,
		ev.Type, nullString(string(ev.TopicID)), formatTime(ev.At), string(ev.Payload))
	if err != nil {
		return wrap("append event", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ev.Seq = seq
	return nil
}

// ListEvents returns up to limit events with Seq greater than after.
func (t *Tx) ListEvents(after int64, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := t.tx.QueryContext(t.ctx, `SELECT seq, type, topic_id, at, payload FROM events WHERE seq > ? ORDER BY seq LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Event
	for rows.Next() {
		var ev domain.Event
		var topic sql.NullString
		var at, payload string
		if err := rows.Scan(&ev.Seq, &ev.Type, &topic, &at, &payload); err != nil {
			return nil, err
		}
		ev.TopicID = domain.ID(topic.String)
		ev.At = parseTime(sql.NullString{String: at, Valid: true})
		ev.Payload = json.RawMessage(payload)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// LastEventSeq returns the newest sequence number, or zero.
func (t *Tx) LastEventSeq() (int64, error) {
	var seq sql.NullInt64
	err := t.tx.QueryRowContext(t.ctx, `SELECT MAX(seq) FROM events`).Scan(&seq)
	return seq.Int64, err
}

// --- helpers --------------------------------------------------------------

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage: %s: %w", op, err)
}

func notFound(entity string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, entity)
	}
	return fmt.Errorf("storage: get %s: %w", entity, err)
}

func affected(entity string, res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, entity)
	}
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nonNil(ids []domain.ID) []domain.ID {
	if ids == nil {
		return []domain.ID{}
	}
	return ids
}
