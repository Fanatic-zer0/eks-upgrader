package state

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteStore is the default local state backend, using a pure-Go SQLite driver in
// WAL mode so that `status`/`report` can read concurrently with an active run.
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLiteStore opens (creating and migrating if necessary) a SQLite-backed Store.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating state directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite state file %q: %w", path, err)
	}

	// WAL mode allows concurrent readers (status/report) while the run holds a writer.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := migrationsFS.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("reading migration %q: %w", name, err)
		}
		if _, err := s.db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("applying migration %q: %w", name, err)
		}
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CreateRun(ctx context.Context, run *Run) error {
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (run_id, cluster, region, source_version, target_version, current_phase, status, pause_reason, abort_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID, run.Cluster, run.Region, run.SourceVersion, run.TargetVersion,
		string(run.CurrentPhase), string(run.Status), run.PauseReason, run.AbortReason,
		run.CreatedAt.Format(time.RFC3339), run.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creating run: %w", err)
	}
	return nil
}

func scanRun(row interface {
	Scan(dest ...interface{}) error
}) (*Run, error) {
	var r Run
	var currentPhase, status, createdAt, updatedAt string
	var pauseReason, abortReason sql.NullString
	if err := row.Scan(&r.RunID, &r.Cluster, &r.Region, &r.SourceVersion, &r.TargetVersion,
		&currentPhase, &status, &pauseReason, &abortReason, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	r.CurrentPhase = Phase(currentPhase)
	r.Status = RunStatus(status)
	r.PauseReason = pauseReason.String
	r.AbortReason = abortReason.String
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &r, nil
}

func (s *SQLiteStore) GetRun(ctx context.Context, runID string) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, cluster, region, source_version, target_version, current_phase, status, pause_reason, abort_reason, created_at, updated_at
		FROM runs WHERE run_id = ?`, runID)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting run %q: %w", runID, err)
	}
	return run, nil
}

func (s *SQLiteStore) LatestRunForCluster(ctx context.Context, cluster string) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, cluster, region, source_version, target_version, current_phase, status, pause_reason, abort_reason, created_at, updated_at
		FROM runs WHERE cluster = ? ORDER BY created_at DESC LIMIT 1`, cluster)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest run for cluster %q: %w", cluster, err)
	}
	return run, nil
}

// LatestRun returns the most recently created run in this state store regardless of
// cluster, used by CLI commands (status/abort/approve/report) that are invoked with
// only --state and no upgrade.yaml/cluster.yaml.
func (s *SQLiteStore) LatestRun(ctx context.Context) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, cluster, region, source_version, target_version, current_phase, status, pause_reason, abort_reason, created_at, updated_at
		FROM runs ORDER BY created_at DESC LIMIT 1`)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest run: %w", err)
	}
	return run, nil
}

func (s *SQLiteStore) UpdateRunPhase(ctx context.Context, runID string, phase Phase, status RunStatus) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET current_phase = ?, status = ?, updated_at = ? WHERE run_id = ?`,
		string(phase), string(status), time.Now().UTC().Format(time.RFC3339), runID)
	if err != nil {
		return fmt.Errorf("updating run phase: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, runID string, status RunStatus, reason string) error {
	var err error
	switch status {
	case StatusPaused, StatusInterrupted:
		_, err = s.db.ExecContext(ctx, `UPDATE runs SET status = ?, pause_reason = ?, updated_at = ? WHERE run_id = ?`,
			string(status), reason, time.Now().UTC().Format(time.RFC3339), runID)
	case StatusAborted:
		_, err = s.db.ExecContext(ctx, `UPDATE runs SET status = ?, abort_reason = ?, updated_at = ? WHERE run_id = ?`,
			string(status), reason, time.Now().UTC().Format(time.RFC3339), runID)
	default:
		_, err = s.db.ExecContext(ctx, `UPDATE runs SET status = ?, updated_at = ? WHERE run_id = ?`,
			string(status), time.Now().UTC().Format(time.RFC3339), runID)
	}
	if err != nil {
		return fmt.Errorf("updating run status: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AppendPhaseHistory(ctx context.Context, entry PhaseHistoryEntry) error {
	var endedAt interface{}
	if entry.EndedAt != nil {
		endedAt = entry.EndedAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO phase_history (run_id, phase, status, started_at, ended_at, detail)
		VALUES (?, ?, ?, ?, ?, ?)`,
		entry.RunID, string(entry.Phase), entry.Status, entry.StartedAt.Format(time.RFC3339), endedAt, entry.Detail)
	if err != nil {
		return fmt.Errorf("appending phase history: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CompletePhaseHistory(ctx context.Context, runID string, phase Phase, status, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE phase_history SET status = ?, ended_at = ?, detail = ?
		WHERE id = (SELECT id FROM phase_history WHERE run_id = ? AND phase = ? ORDER BY id DESC LIMIT 1)`,
		status, time.Now().UTC().Format(time.RFC3339), detail, runID, string(phase))
	if err != nil {
		return fmt.Errorf("completing phase history: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetPhaseHistory(ctx context.Context, runID string) ([]PhaseHistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT phase, status, started_at, ended_at, detail FROM phase_history WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying phase history: %w", err)
	}
	defer rows.Close()

	var out []PhaseHistoryEntry
	for rows.Next() {
		var e PhaseHistoryEntry
		var phase, status, startedAt string
		var endedAt, detail sql.NullString
		if err := rows.Scan(&phase, &status, &startedAt, &endedAt, &detail); err != nil {
			return nil, err
		}
		e.RunID = runID
		e.Phase = Phase(phase)
		e.Status = status
		e.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		if endedAt.Valid {
			t, _ := time.Parse(time.RFC3339, endedAt.String)
			e.EndedAt = &t
		}
		e.Detail = detail.String
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RecordApproval(ctx context.Context, approval Approval) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approvals (run_id, phase, approver, comment, token, approved_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		approval.RunID, string(approval.Phase), approval.Approver, approval.Comment, approval.Token,
		approval.ApprovedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("recording approval: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetApprovals(ctx context.Context, runID string) ([]Approval, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT phase, approver, comment, token, approved_at FROM approvals WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying approvals: %w", err)
	}
	defer rows.Close()

	var out []Approval
	for rows.Next() {
		var a Approval
		var phase, approvedAt string
		var comment sql.NullString
		if err := rows.Scan(&phase, &a.Approver, &comment, &a.Token, &approvedAt); err != nil {
			return nil, err
		}
		a.RunID = runID
		a.Phase = Phase(phase)
		a.Comment = comment.String
		a.ApprovedAt, _ = time.Parse(time.RFC3339, approvedAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RecordEksctlCommand(ctx context.Context, rec EksctlCommandRecord) error {
	argsJSON, err := json.Marshal(rec.Args)
	if err != nil {
		return fmt.Errorf("marshaling eksctl command args: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO eksctl_commands (run_id, phase, args_json, exit_code, duration_ms, started_at, stdout, stderr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.RunID, string(rec.Phase), string(argsJSON), rec.ExitCode, rec.DurationMS,
		rec.StartedAt.Format(time.RFC3339), rec.Stdout, rec.Stderr)
	if err != nil {
		return fmt.Errorf("recording eksctl command: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetEksctlCommands(ctx context.Context, runID string) ([]EksctlCommandRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT phase, args_json, exit_code, duration_ms, started_at, stdout, stderr
		FROM eksctl_commands WHERE run_id = ? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying eksctl commands: %w", err)
	}
	defer rows.Close()

	var out []EksctlCommandRecord
	for rows.Next() {
		var rec EksctlCommandRecord
		var phase, argsJSON, startedAt string
		var stdout, stderr sql.NullString
		if err := rows.Scan(&phase, &argsJSON, &rec.ExitCode, &rec.DurationMS, &startedAt, &stdout, &stderr); err != nil {
			return nil, err
		}
		rec.RunID = runID
		rec.Phase = Phase(phase)
		_ = json.Unmarshal([]byte(argsJSON), &rec.Args)
		rec.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		rec.Stdout = stdout.String
		rec.Stderr = stderr.String
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpsertNodeGroupStatus(ctx context.Context, status NodeGroupStatus) error {
	var startedAt, endedAt interface{}
	if status.StartedAt != nil {
		startedAt = status.StartedAt.Format(time.RFC3339)
	}
	if status.EndedAt != nil {
		endedAt = status.EndedAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_group_status (run_id, name, status, started_at, ended_at, detail)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, name) DO UPDATE SET
			status = excluded.status,
			started_at = COALESCE(node_group_status.started_at, excluded.started_at),
			ended_at = excluded.ended_at,
			detail = excluded.detail`,
		status.RunID, status.Name, status.Status, startedAt, endedAt, status.Detail)
	if err != nil {
		return fmt.Errorf("upserting node group status: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetNodeGroupStatuses(ctx context.Context, runID string) ([]NodeGroupStatus, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, status, started_at, ended_at, detail FROM node_group_status WHERE run_id = ? ORDER BY name ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying node group status: %w", err)
	}
	defer rows.Close()

	var out []NodeGroupStatus
	for rows.Next() {
		var n NodeGroupStatus
		var name, status string
		var startedAt, endedAt, detail sql.NullString
		if err := rows.Scan(&name, &status, &startedAt, &endedAt, &detail); err != nil {
			return nil, err
		}
		n.RunID = runID
		n.Name = name
		n.Status = status
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			n.StartedAt = &t
		}
		if endedAt.Valid {
			t, _ := time.Parse(time.RFC3339, endedAt.String)
			n.EndedAt = &t
		}
		n.Detail = detail.String
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RecordResumeAttempt(ctx context.Context, attempt ResumeAttempt) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO resume_attempts (run_id, phase, attempt, failed_at, error)
		VALUES (?, ?, ?, ?, ?)`,
		attempt.RunID, string(attempt.Phase), attempt.Attempt, attempt.FailedAt.Format(time.RFC3339), attempt.Error)
	if err != nil {
		return fmt.Errorf("recording resume attempt: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CountResumeAttempts(ctx context.Context, runID string, phase Phase) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM resume_attempts WHERE run_id = ? AND phase = ?`, runID, string(phase)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting resume attempts: %w", err)
	}
	return count, nil
}
