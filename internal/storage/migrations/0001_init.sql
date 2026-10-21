-- Initial schema. Timestamps are RFC 3339 strings in UTC; JSON columns hold
-- domain structures encoded by Go, never edited by hand.

CREATE TABLE workspaces (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    root_dir   TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE topics (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    title        TEXT NOT NULL,
    kind         TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX topics_by_workspace ON topics(workspace_id, updated_at DESC);

CREATE TABLE participants (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    name         TEXT NOT NULL,
    provider     TEXT NOT NULL,
    model        TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT '',
    effort       TEXT NOT NULL DEFAULT '',
    max_words    INTEGER NOT NULL DEFAULT 0,
    mode         TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1,
    profile      TEXT NOT NULL,
    tools        TEXT NOT NULL
);
CREATE INDEX participants_by_workspace ON participants(workspace_id);

CREATE TABLE rounds (
    id              TEXT PRIMARY KEY,
    topic_id        TEXT NOT NULL REFERENCES topics(id),
    kind            TEXT NOT NULL,
    status          TEXT NOT NULL,
    prompt          TEXT NOT NULL,
    participant_ids TEXT NOT NULL,  -- JSON array
    inputs          TEXT NOT NULL,  -- JSON FrozenInputs
    auto_reveal     INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    revealed_at     TEXT,
    finished_at     TEXT
);
CREATE INDEX rounds_by_topic ON rounds(topic_id, created_at);
CREATE INDEX rounds_by_status ON rounds(status);

CREATE TABLE runs (
    id                 TEXT PRIMARY KEY,
    topic_id           TEXT NOT NULL REFERENCES topics(id),
    participant_id     TEXT NOT NULL REFERENCES participants(id),
    round_id           TEXT REFERENCES rounds(id),
    trigger_message_id TEXT,
    status             TEXT NOT NULL,
    profile            TEXT NOT NULL,
    tools              TEXT NOT NULL,
    isolation          TEXT NOT NULL,
    work_dir           TEXT NOT NULL DEFAULT '',
    native_session_id  TEXT NOT NULL DEFAULT '',
    result_message_id  TEXT,
    error              TEXT NOT NULL DEFAULT '',
    usage              TEXT NOT NULL DEFAULT '{}',  -- JSON Usage
    created_at         TEXT NOT NULL,
    started_at         TEXT,
    finished_at        TEXT
);
CREATE INDEX runs_by_topic ON runs(topic_id, created_at);
CREATE INDEX runs_by_round ON runs(round_id);
CREATE INDEX runs_by_status ON runs(status);

CREATE TABLE messages (
    id             TEXT PRIMARY KEY,
    topic_id       TEXT NOT NULL REFERENCES topics(id),
    parent_id      TEXT,
    author_kind    TEXT NOT NULL,
    participant_id TEXT,
    body           TEXT NOT NULL,
    mentions       TEXT NOT NULL DEFAULT '[]',  -- JSON array of participant ids
    run_id         TEXT,
    round_id       TEXT,
    visibility     TEXT NOT NULL,
    corrects_id    TEXT,
    created_at     TEXT NOT NULL
);
CREATE INDEX messages_by_topic ON messages(topic_id, created_at, id);
CREATE INDEX messages_by_round ON messages(round_id);

CREATE TABLE context_revisions (
    id             TEXT PRIMARY KEY,
    topic_id       TEXT NOT NULL REFERENCES topics(id),
    version        INTEGER NOT NULL,
    status         TEXT NOT NULL,
    author_kind    TEXT NOT NULL,
    participant_id TEXT,
    run_id         TEXT,
    doc            TEXT NOT NULL,  -- JSON ContextDoc
    created_at     TEXT NOT NULL,
    UNIQUE (topic_id, version)
);

CREATE TABLE artifact_versions (
    id         TEXT PRIMARY KEY,
    topic_id   TEXT NOT NULL REFERENCES topics(id),
    name       TEXT NOT NULL,
    version    INTEGER NOT NULL,
    sha256     TEXT NOT NULL,
    size       INTEGER NOT NULL,
    path       TEXT NOT NULL,
    run_id     TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (topic_id, name, version)
);

CREATE TABLE run_output (
    run_id TEXT NOT NULL REFERENCES runs(id),
    idx    INTEGER NOT NULL,
    text   TEXT NOT NULL,
    PRIMARY KEY (run_id, idx)
);

CREATE TABLE events (
    seq      INTEGER PRIMARY KEY AUTOINCREMENT,
    type     TEXT NOT NULL,
    topic_id TEXT,
    at       TEXT NOT NULL,
    payload  TEXT NOT NULL
);
CREATE INDEX events_by_topic ON events(topic_id, seq);
