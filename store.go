package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store provides a small SQLite-backed persistence layer for sessions, prompts and messages.
type Store struct {
	db *sql.DB
}

// SessionRecord is a lightweight row representation for sessions persisted in the DB.
type SessionRecord struct {
	ID              string
	WorkingDir      string
	Backend         string
	Model           string
	SkipPermissions bool
	ACPSession      string
	Status          string
	Port            int
	ProcID          int64
	LastMessage     string
	CurrentTool     string
	Title           string
	Summary         string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NotificationRecord represents a global notification event persisted for
// mobile clients to poll when background sockets are unavailable.
type NotificationRecord struct {
	ID          int64           `json:"id"`
	EventType   string          `json:"eventType"`
	SessionID   string          `json:"sessionId"`
	SessionName string          `json:"sessionName"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	Meta        json.RawMessage `json:"meta,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// NewStore opens (or creates) the SQLite DB at path and ensures schema exists.
func NewStore(path string) (*Store, error) {
	// Use WAL and a small busy timeout for robustness.
	dsn := path + "?_busy_timeout=5000&_journal_mode=WAL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			working_dir TEXT,
			backend TEXT,
			model TEXT,
			acp_session TEXT,
			status TEXT,
			port INTEGER,
			last_message TEXT,
			current_tool TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			type TEXT,
			data TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			text TEXT,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		);`,

		`CREATE TABLE IF NOT EXISTS processes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			pid INTEGER,
			cmd TEXT,
			args TEXT,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			exited_at DATETIME,
			exit_code INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			session_id TEXT NOT NULL,
			session_name TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			meta TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, fmt.Errorf("setup db: %w", err)
		}
	}

	// Ensure migrations: add missing columns to sessions.
	rows, err := db.Query(`PRAGMA table_info(sessions)`)
	if err == nil {
		defer rows.Close()
		hasProcID := false
		hasSkipPerms := false
		hasTitle := false
		hasSummary := false
		for rows.Next() {
			var cid int
			var name string
			var ctype string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
				if name == "proc_id" {
					hasProcID = true
				}
				if name == "skip_permissions" {
					hasSkipPerms = true
				}
				if name == "title" {
					hasTitle = true
				}
				if name == "summary" {
					hasSummary = true
				}
			}
		}
		if !hasProcID {
			_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN proc_id INTEGER;`)
		}
		if !hasSkipPerms {
			_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN skip_permissions INTEGER DEFAULT 0;`)
		}
		if !hasTitle {
			_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN title TEXT;`)
		}
		if !hasSummary {
			_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN summary TEXT;`)
		}
	}

	// Migration: clear messages stored in old raw-string format (pre-JSON).
	// Old format stored plain strings like "Hello" instead of JSON objects like {"text":"Hello"}.
	// These would cause parse errors on the client, so purge them.
	var needsPurge bool
	row := db.QueryRow(`SELECT data FROM messages LIMIT 1`)
	var sample string
	if row.Scan(&sample) == nil && len(sample) > 0 && sample[0] != '{' {
		needsPurge = true
	}
	if needsPurge {
		_, _ = db.Exec(`DELETE FROM messages`)
	}

	// Ensure notifications table has a 'meta' column for structured context (added in a later update).
	rows2, err := db.Query(`PRAGMA table_info(notifications)`)
	if err == nil {
		defer rows2.Close()
		hasMeta := false
		for rows2.Next() {
			var cid int
			var name string
			var ctype string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := rows2.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
				if name == "meta" {
					hasMeta = true
				}
			}
		}
		if !hasMeta {
			_, _ = db.Exec(`ALTER TABLE notifications ADD COLUMN meta TEXT;`)
		}
	}

	return &Store{db: db}, nil
}

func (st *Store) Close() error {
	return st.db.Close()
}

// UpsertSession inserts or updates a session row from an in-memory Session.
func (st *Store) UpsertSession(s *Session) error {
	s.summaryMu.RLock()
	lastMessage := s.lastMessage
	currentTool := s.currentTool
	title := s.title
	summary := s.summary
	s.summaryMu.RUnlock()

	skipPerms := 0
	if s.SkipPermissions {
		skipPerms = 1
	}
	_, err := st.db.Exec(
		`INSERT INTO sessions (id, working_dir, backend, model, skip_permissions, acp_session, status, port, proc_id, last_message, current_tool, title, summary, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   working_dir=excluded.working_dir,
		   backend=excluded.backend,
		   model=excluded.model,
		   skip_permissions=excluded.skip_permissions,
		   acp_session=excluded.acp_session,
		   status=excluded.status,
		   port=excluded.port,
		   proc_id=excluded.proc_id,
		   last_message=excluded.last_message,
		   current_tool=excluded.current_tool,
		   title=excluded.title,
		   summary=excluded.summary,
		   updated_at=CURRENT_TIMESTAMP;`,
		s.ID, s.WorkingDir, s.Backend, s.Model, skipPerms, s.ACPSession, s.Status, s.port, s.procID, lastMessage, currentTool, title, summary,
	)
	return err
}

// DeleteSession removes a session and all its related data from the DB.
func (st *Store) UpdateSessionSummary(sessionID, title, summary string) error {
	_, err := st.db.Exec(
		`UPDATE sessions SET title=?, summary=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		title, summary, sessionID,
	)
	return err
}

func (st *Store) DeleteSession(sessionID string) error {
	tx, err := st.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM messages WHERE session_id = ?`,
		`DELETE FROM prompts WHERE session_id = ?`,
		`DELETE FROM processes WHERE session_id = ?`,
		`DELETE FROM sessions WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, sessionID); err != nil {
			return fmt.Errorf("delete session data: %w", err)
		}
	}
	return tx.Commit()
}

// LoadSessions returns all persisted sessions.
func (st *Store) LoadSessions() ([]SessionRecord, error) {
	rows, err := st.db.Query(`SELECT id, working_dir, backend, model, skip_permissions, acp_session, status, port, proc_id, last_message, current_tool, title, summary, created_at FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var skipPerms sql.NullInt64
		var port sql.NullInt64
		var proc sql.NullInt64
		var lastMsg sql.NullString
		var curTool sql.NullString
		var title sql.NullString
		var summary sql.NullString
		var createdAt sql.NullString
		if err := rows.Scan(&r.ID, &r.WorkingDir, &r.Backend, &r.Model, &skipPerms, &r.ACPSession, &r.Status, &port, &proc, &lastMsg, &curTool, &title, &summary, &createdAt); err != nil {
			return nil, err
		}
		r.SkipPermissions = skipPerms.Valid && skipPerms.Int64 != 0
		if port.Valid {
			r.Port = int(port.Int64)
		}
		if proc.Valid {
			r.ProcID = proc.Int64
		}
		if lastMsg.Valid {
			r.LastMessage = lastMsg.String
		}
		if curTool.Valid {
			r.CurrentTool = curTool.String
		}
		if title.Valid {
			r.Title = title.String
		}
		if summary.Valid {
			r.Summary = summary.String
		}
		if createdAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
				r.CreatedAt = t
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// LoadMessages returns persisted messages for a session, used to seed Hub history on restart.
// Messages are stored as JSON strings matching the broadcast format, so we pass them through as raw JSON.
func (st *Store) LoadMessages(sessionID string) ([]WSMessage, error) {
	rows, err := st.db.Query(`SELECT type, data FROM messages WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WSMessage
	for rows.Next() {
		var typ, data string
		if err := rows.Scan(&typ, &data); err != nil {
			return nil, err
		}
		out = append(out, WSMessage{Type: typ, Data: json.RawMessage(data)})
	}
	return out, nil
}

// SaveMessage appends a message row for a session (used for history persistence).
// data must be a valid JSON string matching the broadcast format the client expects.
func (st *Store) SaveMessage(sessionID, typ, data string) error {
	_, err := st.db.Exec(`INSERT INTO messages (session_id, type, data) VALUES (?, ?, ?)`, sessionID, typ, data)
	return err
}

// SaveMessageJSON marshals the given value to JSON and stores it. Use the same struct
// passed to hub.Broadcast so the stored format matches what clients expect on replay.
func (st *Store) SaveMessageJSON(sessionID, typ string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return st.SaveMessage(sessionID, typ, string(raw))
}

// InsertPrompt records a new prompt and returns its id.
func (st *Store) InsertPrompt(sessionID, text string) (int64, error) {
	res, err := st.db.Exec(`INSERT INTO prompts (session_id, text, status) VALUES (?, ?, 'pending')`, sessionID, text)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePromptStatus updates prompt status and sets completed_at when done or error.
func (st *Store) UpdatePromptStatus(promptID int64, status string) error {
	if status == "done" || status == "error" {
		_, err := st.db.Exec(`UPDATE prompts SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`, status, promptID)
		return err
	}
	_, err := st.db.Exec(`UPDATE prompts SET status = ? WHERE id = ?`, status, promptID)
	return err
}

// RecordProcessStart inserts a process record and returns its row id.
func (st *Store) RecordProcessStart(sessionID string, pid int, cmd string, args string) (int64, error) {
	res, err := st.db.Exec(`INSERT INTO processes (session_id, pid, cmd, args) VALUES (?, ?, ?, ?)`, sessionID, pid, cmd, args)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateProcessExit updates the process exit info.
func (st *Store) UpdateProcessExit(procID int64, exitCode int) error {
	_, err := st.db.Exec(`UPDATE processes SET exited_at = CURRENT_TIMESTAMP, exit_code = ? WHERE id = ?`, exitCode, procID)
	return err
}

// GetProcess returns basic process metadata.
func (st *Store) GetProcess(procID int64) (map[string]any, error) {
	row := st.db.QueryRow(`SELECT id, session_id, pid, cmd, args, started_at, exited_at, exit_code FROM processes WHERE id = ?`, procID)
	var id int64
	var sessionID, cmd, args sql.NullString
	var pid sql.NullInt64
	var startedAt, exitedAt sql.NullString
	var exitCode sql.NullInt64
	if err := row.Scan(&id, &sessionID, &pid, &cmd, &args, &startedAt, &exitedAt, &exitCode); err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":        id,
		"sessionId": sessionID.String,
		"pid":       int(pid.Int64),
		"cmd":       cmd.String,
		"args":      args.String,
		"startedAt": startedAt.String,
		"exitedAt":  exitedAt.String,
		"exitCode":  int(exitCode.Int64),
	}
	return out, nil
}

// InsertNotification stores a notification event and returns its ID.
func (st *Store) InsertNotification(eventType, sessionID, sessionName, title, body string, meta json.RawMessage) (int64, error) {
	// meta may be nil/empty; store as NULL if so
	if len(meta) == 0 {
		res, err := st.db.Exec(
			`INSERT INTO notifications (event_type, session_id, session_name, title, body) VALUES (?, ?, ?, ?, ?)`,
			eventType, sessionID, sessionName, title, body,
		)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := st.db.Exec(
		`INSERT INTO notifications (event_type, session_id, session_name, title, body, meta) VALUES (?, ?, ?, ?, ?, ?)`,
		eventType, sessionID, sessionName, title, body, string(meta),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListNotificationsAfter returns global notifications with id > afterID.
func (st *Store) ListNotificationsAfter(afterID int64, limit int) ([]NotificationRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := st.db.Query(
		`SELECT id, event_type, session_id, session_name, title, body, meta, created_at
		 FROM notifications
		 WHERE id > ?
		 ORDER BY id ASC
		 LIMIT ?`,
		afterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NotificationRecord, 0)
	for rows.Next() {
		var rec NotificationRecord
		var createdAtRaw string
		var metaRaw sql.NullString
		if err := rows.Scan(
			&rec.ID,
			&rec.EventType,
			&rec.SessionID,
			&rec.SessionName,
			&rec.Title,
			&rec.Body,
			&metaRaw,
			&createdAtRaw,
		); err != nil {
			return nil, err
		}
		if metaRaw.Valid {
			rec.Meta = json.RawMessage(metaRaw.String)
		}
		if ts, err := time.Parse("2006-01-02 15:04:05", createdAtRaw); err == nil {
			rec.CreatedAt = ts
		} else {
			rec.CreatedAt = time.Now()
		}
		out = append(out, rec)
	}
	return out, nil
}
