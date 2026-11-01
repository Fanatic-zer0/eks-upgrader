CREATE TABLE IF NOT EXISTS runs (
    run_id         TEXT PRIMARY KEY,
    cluster        TEXT NOT NULL,
    region         TEXT NOT NULL,
    source_version TEXT NOT NULL,
    target_version TEXT NOT NULL,
    current_phase  TEXT NOT NULL,
    status         TEXT NOT NULL,
    pause_reason   TEXT,
    abort_reason   TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS phase_history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     TEXT NOT NULL REFERENCES runs(run_id),
    phase      TEXT NOT NULL,
    status     TEXT NOT NULL,
    started_at TEXT NOT NULL,
    ended_at   TEXT,
    detail     TEXT
);
CREATE INDEX IF NOT EXISTS idx_phase_history_run ON phase_history(run_id);

CREATE TABLE IF NOT EXISTS approvals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT NOT NULL REFERENCES runs(run_id),
    phase       TEXT NOT NULL,
    approver    TEXT NOT NULL,
    comment     TEXT,
    token       TEXT NOT NULL,
    approved_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_approvals_run ON approvals(run_id);

CREATE TABLE IF NOT EXISTS eksctl_commands (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT NOT NULL REFERENCES runs(run_id),
    phase       TEXT NOT NULL,
    args_json   TEXT NOT NULL,
    exit_code   INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    started_at  TEXT NOT NULL,
    stdout      TEXT,
    stderr      TEXT
);
CREATE INDEX IF NOT EXISTS idx_eksctl_commands_run ON eksctl_commands(run_id);

CREATE TABLE IF NOT EXISTS node_group_status (
    run_id     TEXT NOT NULL REFERENCES runs(run_id),
    name       TEXT NOT NULL,
    status     TEXT NOT NULL,
    started_at TEXT,
    ended_at   TEXT,
    detail     TEXT,
    PRIMARY KEY (run_id, name)
);

CREATE TABLE IF NOT EXISTS resume_attempts (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id    TEXT NOT NULL REFERENCES runs(run_id),
    phase     TEXT NOT NULL,
    attempt   INTEGER NOT NULL,
    failed_at TEXT NOT NULL,
    error     TEXT
);
CREATE INDEX IF NOT EXISTS idx_resume_attempts_run_phase ON resume_attempts(run_id, phase);
