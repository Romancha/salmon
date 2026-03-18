package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/romancha/salmon/internal/models"

	_ "modernc.org/sqlite"
)

const (
	syncStatusConflict      = "conflict"
	syncStatusPendingToBear = "pending_to_bear"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens a SQLite database at dbPath and runs migrations.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}

	// Serialize all writes through a single connection to avoid SQLite locking contention.
	// WAL mode allows concurrent reads but serializes writes; using one connection avoids
	// busy_timeout delays under concurrent API load.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// DB returns the underlying *sql.DB for use in tests.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close db: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	ddl := `
CREATE TABLE IF NOT EXISTS notes (
    rowid               INTEGER PRIMARY KEY AUTOINCREMENT,
    id                  TEXT NOT NULL UNIQUE,
    bear_id             TEXT UNIQUE,
    title               TEXT NOT NULL DEFAULT '',
    subtitle            TEXT DEFAULT '',
    body                TEXT NOT NULL DEFAULT '',
    archived            INTEGER DEFAULT 0,
    encrypted           INTEGER DEFAULT 0,
    has_files           INTEGER DEFAULT 0,
    has_images          INTEGER DEFAULT 0,
    has_source_code     INTEGER DEFAULT 0,
    locked              INTEGER DEFAULT 0,
    pinned              INTEGER DEFAULT 0,
    shown_in_today      INTEGER DEFAULT 0,
    trashed             INTEGER DEFAULT 0,
    permanently_deleted INTEGER DEFAULT 0,
    skip_sync           INTEGER DEFAULT 0,
    todo_completed      INTEGER DEFAULT 0,
    todo_incompleted    INTEGER DEFAULT 0,
    version             INTEGER DEFAULT 0,
    created_at          TEXT,
    modified_at         TEXT,
    archived_at         TEXT,
    encrypted_at        TEXT,
    locked_at           TEXT,
    pinned_at           TEXT,
    trashed_at          TEXT,
    order_date          TEXT,
    conflict_id_date    TEXT,
    last_editing_device TEXT,
    conflict_id         TEXT,
    encryption_id       TEXT,
    encrypted_data      BLOB,
    sync_status         TEXT DEFAULT 'synced',
    hub_modified_at     TEXT,
    bear_raw            TEXT,
    pending_bear_title  TEXT,
    pending_bear_body   TEXT,
    expected_bear_modified_at TEXT
);
CREATE TABLE IF NOT EXISTS tags (
    id                      TEXT PRIMARY KEY,
    bear_id                 TEXT UNIQUE,
    title                   TEXT NOT NULL UNIQUE,
    pinned                  INTEGER DEFAULT 0,
    is_root                 INTEGER DEFAULT 0,
    hide_subtags_notes      INTEGER DEFAULT 0,
    sorting                 INTEGER DEFAULT 0,
    sorting_direction       INTEGER DEFAULT 0,
    encrypted               INTEGER DEFAULT 0,
    version                 INTEGER DEFAULT 0,
    modified_at             TEXT,
    pinned_at               TEXT,
    pinned_notes_at         TEXT,
    encrypted_at            TEXT,
    hide_subtags_notes_at   TEXT,
    sorting_at              TEXT,
    sorting_direction_at    TEXT,
    tag_con_date            TEXT,
    tag_con                 TEXT,
    bear_raw                TEXT
);
CREATE TABLE IF NOT EXISTS note_tags (
    note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (note_id, tag_id)
);
CREATE TABLE IF NOT EXISTS pinned_note_tags (
    note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (note_id, tag_id)
);
CREATE TABLE IF NOT EXISTS attachments (
    id                      TEXT PRIMARY KEY,
    bear_id                 TEXT UNIQUE,
    note_id                 TEXT REFERENCES notes(id) ON DELETE CASCADE,
    type                    TEXT NOT NULL,
    filename                TEXT,
    normalized_extension    TEXT,
    file_size               INTEGER,
    file_index              INTEGER,
    width                   INTEGER,
    height                  INTEGER,
    animated                INTEGER DEFAULT 0,
    duration                INTEGER,
    width1                  INTEGER,
    height1                 INTEGER,
    downloaded              INTEGER DEFAULT 0,
    encrypted               INTEGER DEFAULT 0,
    permanently_deleted     INTEGER DEFAULT 0,
    skip_sync               INTEGER DEFAULT 0,
    unused                  INTEGER DEFAULT 0,
    uploaded                INTEGER DEFAULT 0,
    version                 INTEGER DEFAULT 0,
    created_at              TEXT,
    modified_at             TEXT,
    inserted_at             TEXT,
    encrypted_at            TEXT,
    unused_at               TEXT,
    uploaded_at             TEXT,
    search_text_at          TEXT,
    last_editing_device     TEXT,
    encryption_id           TEXT,
    search_text             TEXT,
    encrypted_data          BLOB,
    file_path               TEXT,
    bear_raw                TEXT
);
CREATE TABLE IF NOT EXISTS backlinks (
    id              TEXT PRIMARY KEY,
    bear_id         TEXT UNIQUE,
    linked_by_id    TEXT REFERENCES notes(id) ON DELETE CASCADE,
    linking_to_id   TEXT REFERENCES notes(id) ON DELETE CASCADE,
    title           TEXT,
    location        INTEGER,
    version         INTEGER DEFAULT 0,
    modified_at     TEXT,
    bear_raw        TEXT
);
CREATE TABLE IF NOT EXISTS write_queue (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    idempotency_key TEXT NOT NULL,
    action          TEXT NOT NULL,
    note_id         TEXT,
    payload         TEXT NOT NULL,
    created_at      TEXT DEFAULT (datetime('now')),
    status          TEXT DEFAULT 'pending',
    processing_by   TEXT,
    lease_until     TEXT,
    applied_at      TEXT,
    error           TEXT,
    consumer_id     TEXT NOT NULL DEFAULT '',
    UNIQUE(idempotency_key, consumer_id)
);
CREATE TABLE IF NOT EXISTS sync_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notes_modified ON notes(modified_at);
CREATE INDEX IF NOT EXISTS idx_notes_trashed ON notes(trashed);
CREATE INDEX IF NOT EXISTS idx_notes_sync_status ON notes(sync_status);
CREATE INDEX IF NOT EXISTS idx_attachments_note ON attachments(note_id);
CREATE INDEX IF NOT EXISTS idx_backlinks_linked_by ON backlinks(linked_by_id);
CREATE INDEX IF NOT EXISTS idx_backlinks_linking_to ON backlinks(linking_to_id);
CREATE INDEX IF NOT EXISTS idx_write_queue_status ON write_queue(status);
CREATE INDEX IF NOT EXISTS idx_write_queue_lease ON write_queue(status, lease_until);
`

	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("execute DDL: %w", err)
	}

	var ftsCount int

	err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='notes_fts'",
	).Scan(&ftsCount)
	if err != nil {
		return fmt.Errorf("check notes_fts: %w", err)
	}

	if ftsCount == 0 {
		ftsDDL := `
CREATE VIRTUAL TABLE notes_fts USING fts5(title, body, content='notes', content_rowid='rowid');
CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
CREATE TRIGGER notes_fts_update AFTER UPDATE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
    INSERT INTO notes_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
    INSERT INTO notes_fts(notes_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
END;
`
		if _, err := s.db.ExecContext(ctx, ftsDDL); err != nil {
			return fmt.Errorf("create FTS5: %w", err)
		}
	}

	// Migrate existing write_queue tables: add consumer_id column if missing.
	if err := s.migrateWriteQueueConsumerID(ctx); err != nil {
		return fmt.Errorf("migrate write_queue consumer_id: %w", err)
	}

	// Migrate existing notes table: add pending_bear_title/body columns if missing.
	if err := s.migratePendingBearColumns(ctx); err != nil {
		return fmt.Errorf("migrate pending_bear columns: %w", err)
	}

	return nil
}

// migratePendingBearColumns adds pending_bear_title and pending_bear_body columns to an existing
// notes table for field-level conflict detection. These columns store Bear's title/body snapshot
// at enqueue time, so conflict detection can compare per-field instead of using timestamps.
func (s *SQLiteStore) migratePendingBearColumns(ctx context.Context) error {
	var ddlSQL sql.NullString
	if err := s.db.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'notes'",
	).Scan(&ddlSQL); err != nil {
		return fmt.Errorf("read notes DDL: %w", err)
	}

	ddl := ""
	if ddlSQL.Valid {
		ddl = ddlSQL.String
	}

	if !strings.Contains(ddl, "pending_bear_title") {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE notes ADD COLUMN pending_bear_title TEXT"); err != nil {
			return fmt.Errorf("add pending_bear_title column: %w", err)
		}
	}

	if !strings.Contains(ddl, "pending_bear_body") {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE notes ADD COLUMN pending_bear_body TEXT"); err != nil {
			return fmt.Errorf("add pending_bear_body column: %w", err)
		}
	}

	if !strings.Contains(ddl, "expected_bear_modified_at") {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE notes ADD COLUMN expected_bear_modified_at TEXT"); err != nil {
			return fmt.Errorf("add expected_bear_modified_at column: %w", err)
		}
	}

	return nil
}

// migrateWriteQueueConsumerID adds the consumer_id column to an existing write_queue table
// and replaces the old idempotency_key UNIQUE index with a compound (idempotency_key, consumer_id) one.
// SQLite doesn't support DROP CONSTRAINT, so we recreate the table to change the uniqueness semantics.
//
// The check inspects the actual table DDL in sqlite_master rather than just column existence,
// because an intermediate schema version may have added consumer_id without changing the
// UNIQUE constraint from UNIQUE(idempotency_key) to UNIQUE(idempotency_key, consumer_id).
func (s *SQLiteStore) migrateWriteQueueConsumerID(ctx context.Context) error {
	// Check whether the table already has the correct compound unique constraint.
	// We look for "UNIQUE(idempotency_key, consumer_id)" in the DDL stored in sqlite_master.
	var ddlSQL sql.NullString
	if err := s.db.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'write_queue'",
	).Scan(&ddlSQL); err != nil {
		return fmt.Errorf("read write_queue DDL: %w", err)
	}

	if ddlSQL.Valid && strings.Contains(ddlSQL.String, "UNIQUE(idempotency_key, consumer_id)") {
		return nil // already has the correct compound unique constraint
	}

	// Recreate the table with consumer_id column and compound unique constraint.
	// Wrapped in an explicit transaction to prevent partial state on crash.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write_queue migration tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Determine the consumer_id source expression: if the old table already has a consumer_id
	// column (intermediate schema), preserve existing values; otherwise default to ''.
	consumerIDExpr := "''"
	if ddlSQL.Valid && strings.Contains(ddlSQL.String, "consumer_id") {
		consumerIDExpr = "COALESCE(consumer_id, '')"
	}

	//nolint:gosec // consumerIDExpr is a hardcoded constant, not user input
	migrate := fmt.Sprintf(`
CREATE TABLE write_queue_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    idempotency_key TEXT NOT NULL,
    action          TEXT NOT NULL,
    note_id         TEXT,
    payload         TEXT NOT NULL,
    created_at      TEXT DEFAULT (datetime('now')),
    status          TEXT DEFAULT 'pending',
    processing_by   TEXT,
    lease_until     TEXT,
    applied_at      TEXT,
    error           TEXT,
    consumer_id     TEXT NOT NULL DEFAULT '',
    UNIQUE(idempotency_key, consumer_id)
);
INSERT INTO write_queue_new (id, idempotency_key, action, note_id, payload, created_at, status,
    processing_by, lease_until, applied_at, error, consumer_id)
SELECT id, idempotency_key, action, note_id, payload, created_at, status,
    processing_by, lease_until, applied_at, error, %s
FROM write_queue;
DROP TABLE write_queue;
ALTER TABLE write_queue_new RENAME TO write_queue;
CREATE INDEX IF NOT EXISTS idx_write_queue_status ON write_queue(status);
CREATE INDEX IF NOT EXISTS idx_write_queue_lease ON write_queue(status, lease_until);
`, consumerIDExpr)

	if _, err := tx.ExecContext(ctx, migrate); err != nil {
		return fmt.Errorf("migrate write_queue uniqueness: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit write_queue migration: %w", err)
	}

	return nil
}

// --- Notes ---

//nolint:gocritic // intentional value receiver for simple filter struct
func (s *SQLiteStore) ListNotes(ctx context.Context, filter NoteFilter) ([]models.Note, error) {
	// Always exclude permanently-deleted and archived notes from the API listing.
	// These are hidden in Bear's UI and should not be surfaced to consumers.
	where := []string{"n.permanently_deleted = 0", "n.archived = 0"}
	var args []any

	if filter.Tag != "" {
		where = append(where,
			"n.id IN (SELECT nt.note_id FROM note_tags nt "+
				"JOIN tags t ON nt.tag_id = t.id WHERE t.title = ?)")
		args = append(args, filter.Tag)
	}

	if filter.Trashed != nil {
		if *filter.Trashed {
			where = append(where, "n.trashed = 1")
		} else {
			where = append(where, "n.trashed = 0")
		}
	}

	if filter.Encrypted != nil {
		if *filter.Encrypted {
			where = append(where, "n.encrypted = 1")
		} else {
			where = append(where, "n.encrypted = 0")
		}
	}

	query := "SELECT " + noteColumnsWithPrefix("n.") + " FROM notes n" //nolint:gosec // columns are internal constants
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	sortCol := "n.modified_at"
	if filter.Sort != "" {
		allowed := map[string]bool{
			"modified_at": true, "created_at": true, "title": true,
		}
		if allowed[filter.Sort] {
			sortCol = "n." + filter.Sort
		}
	}

	order := "DESC"
	if filter.Order == "asc" {
		order = "ASC"
	}

	query += " ORDER BY " + sortCol + " " + order

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var notes []models.Note

	for rows.Next() {
		n, scanErr := scanNote(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan note: %w", scanErr)
		}

		notes = append(notes, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notes: %w", err)
	}

	for i := range notes {
		tags, tagErr := s.loadNoteTags(ctx, notes[i].ID)
		if tagErr != nil {
			return nil, fmt.Errorf("load tags for note %s: %w", notes[i].ID, tagErr)
		}

		notes[i].Tags = tags
	}

	return notes, nil
}

func (s *SQLiteStore) GetNote(ctx context.Context, id string) (*models.Note, error) {
	query := "SELECT " + noteColumns() + " FROM notes WHERE id = ?"

	n, err := scanNoteRow(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get note: %w", err)
	}

	tags, err := s.loadNoteTags(ctx, n.ID)
	if err != nil {
		return nil, fmt.Errorf("load tags for note %s: %w", n.ID, err)
	}

	n.Tags = tags

	backlinks, err := s.ListBacklinksByNote(ctx, n.ID)
	if err != nil {
		return nil, fmt.Errorf("load backlinks for note %s: %w", n.ID, err)
	}

	n.Backlinks = backlinks

	return &n, nil
}

func (s *SQLiteStore) CreateNote(ctx context.Context, note *models.Note) error {
	query := `INSERT INTO notes (
		id, bear_id, title, subtitle, body,
		archived, encrypted, has_files, has_images, has_source_code,
		locked, pinned, shown_in_today, trashed, permanently_deleted,
		skip_sync, todo_completed, todo_incompleted, version,
		created_at, modified_at, archived_at, encrypted_at, locked_at,
		pinned_at, trashed_at, order_date, conflict_id_date,
		last_editing_device, conflict_id, encryption_id, encrypted_data,
		sync_status, hub_modified_at, bear_raw,
		pending_bear_title, pending_bear_body,
		expected_bear_modified_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?
	)`

	if _, err := s.db.ExecContext(ctx, query, noteValues(note)...); err != nil {
		return fmt.Errorf("create note: %w", err)
	}

	return nil
}

func (s *SQLiteStore) UpdateNote(ctx context.Context, note *models.Note) error {
	query := `UPDATE notes SET
		bear_id = ?, title = ?, subtitle = ?, body = ?,
		archived = ?, encrypted = ?, has_files = ?, has_images = ?,
		has_source_code = ?, locked = ?, pinned = ?, shown_in_today = ?,
		trashed = ?, permanently_deleted = ?, skip_sync = ?,
		todo_completed = ?, todo_incompleted = ?, version = ?,
		created_at = ?, modified_at = ?, archived_at = ?, encrypted_at = ?,
		locked_at = ?, pinned_at = ?, trashed_at = ?, order_date = ?,
		conflict_id_date = ?, last_editing_device = ?, conflict_id = ?,
		encryption_id = ?, encrypted_data = ?, sync_status = ?,
		hub_modified_at = ?, bear_raw = ?,
		pending_bear_title = ?, pending_bear_body = ?,
		expected_bear_modified_at = ?
	WHERE id = ?`

	vals := noteValues(note)
	vals = append(vals[1:], vals[0])

	if _, err := s.db.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update note: %w", err)
	}

	return nil
}

// DeleteNote removes a note by its ID.
func (s *SQLiteStore) DeleteNote(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM notes WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	return nil
}

// --- FTS5 Search ---

func (s *SQLiteStore) SearchNotes(
	ctx context.Context, query string, tag string, limit int,
) ([]models.Note, error) {
	if limit <= 0 {
		limit = 20
	}

	var where []string
	var args []any

	// Sanitize FTS5 query: wrap in double quotes to treat as phrase search,
	// escaping any embedded double quotes to prevent FTS5 syntax injection.
	sanitized := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	where = append(where, "notes_fts MATCH ?")
	args = append(args, sanitized)

	if tag != "" {
		where = append(where,
			"n.id IN (SELECT nt.note_id FROM note_tags nt "+
				"JOIN tags t ON nt.tag_id = t.id WHERE t.title = ?)")
		args = append(args, tag)
	}

	//nolint:gosec // columns and where clauses are internal constants
	q := "SELECT " + noteColumnsWithPrefix("n.") +
		" FROM notes n JOIN notes_fts ON n.rowid = notes_fts.rowid" +
		" WHERE " + strings.Join(where, " AND ") +
		" ORDER BY rank" + fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var notes []models.Note

	for rows.Next() {
		n, scanErr := scanNote(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan search result: %w", scanErr)
		}

		notes = append(notes, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	for i := range notes {
		tags, tagErr := s.loadNoteTags(ctx, notes[i].ID)
		if tagErr != nil {
			return nil, fmt.Errorf("load tags for note %s: %w", notes[i].ID, tagErr)
		}

		notes[i].Tags = tags
	}

	return notes, nil
}

// --- Tags ---

func (s *SQLiteStore) ListTags(ctx context.Context) ([]models.Tag, error) {
	query := "SELECT " + tagColumns() + " FROM tags ORDER BY title" //nolint:gosec // columns are internal constants

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var tags []models.Tag

	for rows.Next() {
		t, scanErr := scanTag(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan tag: %w", scanErr)
		}

		tags = append(tags, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return tags, nil
}

func (s *SQLiteStore) GetTag(ctx context.Context, id string) (*models.Tag, error) {
	query := "SELECT " + tagColumns() + " FROM tags WHERE id = ?"

	t, err := scanTagRow(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get tag: %w", err)
	}

	return &t, nil
}

func (s *SQLiteStore) CreateTag(ctx context.Context, tag *models.Tag) error {
	query := `INSERT INTO tags (
		id, bear_id, title, pinned, is_root, hide_subtags_notes,
		sorting, sorting_direction, encrypted, version,
		modified_at, pinned_at, pinned_notes_at, encrypted_at,
		hide_subtags_notes_at, sorting_at, sorting_direction_at,
		tag_con_date, tag_con, bear_raw
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.ExecContext(ctx, query, tagValues(tag)...); err != nil {
		return fmt.Errorf("create tag: %w", err)
	}

	return nil
}

// --- Attachments ---

func (s *SQLiteStore) GetAttachment(ctx context.Context, id string) (*models.Attachment, error) {
	query := "SELECT " + attachmentColumns() + " FROM attachments WHERE id = ?"

	a, err := scanAttachmentRow(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get attachment: %w", err)
	}

	return &a, nil
}

func (s *SQLiteStore) GetAttachmentByBearID(ctx context.Context, bearID string) (*models.Attachment, error) {
	query := "SELECT " + attachmentColumns() + " FROM attachments WHERE bear_id = ?"

	a, err := scanAttachmentRow(s.db.QueryRowContext(ctx, query, bearID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get attachment by bear_id: %w", err)
	}

	return &a, nil
}

func (s *SQLiteStore) ListAttachmentsByNote(
	ctx context.Context, noteID string,
) ([]models.Attachment, error) {
	//nolint:gosec // columns are internal constants
	query := "SELECT " + attachmentColumns() + " FROM attachments WHERE note_id = ?"

	rows, err := s.db.QueryContext(ctx, query, noteID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var attachments []models.Attachment

	for rows.Next() {
		a, scanErr := scanAttachment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan attachment: %w", scanErr)
		}

		attachments = append(attachments, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachments: %w", err)
	}

	return attachments, nil
}

func (s *SQLiteStore) UpdateAttachment(ctx context.Context, a *models.Attachment) error {
	query := `UPDATE attachments SET
		bear_id = ?, note_id = ?, type = ?, filename = ?,
		normalized_extension = ?, file_size = ?, file_index = ?,
		width = ?, height = ?, animated = ?, duration = ?,
		width1 = ?, height1 = ?, downloaded = ?, encrypted = ?,
		permanently_deleted = ?, skip_sync = ?, unused = ?,
		uploaded = ?, version = ?, created_at = ?, modified_at = ?,
		inserted_at = ?, encrypted_at = ?, unused_at = ?,
		uploaded_at = ?, search_text_at = ?, last_editing_device = ?,
		encryption_id = ?, search_text = ?, encrypted_data = ?,
		file_path = ?, bear_raw = ?
	WHERE id = ?`

	vals := attachmentValues(a)
	vals = append(vals[1:], vals[0])

	if _, err := s.db.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update attachment: %w", err)
	}

	return nil
}

// --- Backlinks ---

func (s *SQLiteStore) ListBacklinksByNote(
	ctx context.Context, noteID string,
) ([]models.Backlink, error) {
	//nolint:gosec // columns are internal constants
	query := "SELECT " + backlinkColumns() + " FROM backlinks WHERE linking_to_id = ?"

	rows, err := s.db.QueryContext(ctx, query, noteID)
	if err != nil {
		return nil, fmt.Errorf("list backlinks: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var backlinks []models.Backlink

	for rows.Next() {
		b, scanErr := scanBacklink(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan backlink: %w", scanErr)
		}

		backlinks = append(backlinks, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backlinks: %w", err)
	}

	return backlinks, nil
}

// --- Sync Push ---

//nolint:gocognit,gocritic // sync push handles multiple entity types; value receiver is intentional
func (s *SQLiteStore) ProcessSyncPush(ctx context.Context, req models.SyncPushRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync push tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for i := range req.Notes {
		if err := upsertNote(ctx, tx, &req.Notes[i]); err != nil {
			return fmt.Errorf("upsert note: %w", err)
		}
	}

	for i := range req.Tags {
		if err := upsertTag(ctx, tx, &req.Tags[i]); err != nil {
			return fmt.Errorf("upsert tag: %w", err)
		}
	}

	if err := replaceNoteTags(ctx, tx, "note_tags", req.Notes, req.NoteTags); err != nil {
		return fmt.Errorf("replace note_tags: %w", err)
	}

	if err := replaceNoteTags(ctx, tx, "pinned_note_tags", req.Notes, req.PinnedNoteTags); err != nil {
		return fmt.Errorf("replace pinned_note_tags: %w", err)
	}

	for i := range req.Attachments {
		if err := upsertAttachment(ctx, tx, &req.Attachments[i]); err != nil {
			return fmt.Errorf("upsert attachment: %w", err)
		}
	}

	for i := range req.Backlinks {
		if err := upsertBacklink(ctx, tx, &req.Backlinks[i]); err != nil {
			return fmt.Errorf("upsert backlink: %w", err)
		}
	}

	if err := failQueueItemsForDeletedNotes(ctx, tx, req.DeletedNoteIDs); err != nil {
		return fmt.Errorf("fail queue items for deleted notes: %w", err)
	}

	if err := deleteByBearIDs(ctx, tx, "notes", req.DeletedNoteIDs); err != nil {
		return fmt.Errorf("delete notes: %w", err)
	}

	if err := deleteByBearIDs(ctx, tx, "tags", req.DeletedTagIDs); err != nil {
		return fmt.Errorf("delete tags: %w", err)
	}

	if err := deleteByBearIDs(ctx, tx, "attachments", req.DeletedAttachmentIDs); err != nil {
		return fmt.Errorf("delete attachments: %w", err)
	}

	if err := deleteByBearIDs(ctx, tx, "backlinks", req.DeletedBacklinkIDs); err != nil {
		return fmt.Errorf("delete backlinks: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync push: %w", err)
	}

	return nil
}

func upsertNote(ctx context.Context, tx *sql.Tx, note *models.Note) error {
	var existingID string
	var existingSyncStatus string
	var existingModifiedAt string
	var existingTitle, existingBody string
	var pendingBearTitle, pendingBearBody sql.NullString
	var expectedBearModifiedAt sql.NullString

	if note.BearID != nil && *note.BearID != "" {
		err := tx.QueryRowContext(ctx,
			`SELECT id, sync_status, COALESCE(modified_at, ''),
				COALESCE(title, ''), COALESCE(body, ''),
				pending_bear_title, pending_bear_body,
				expected_bear_modified_at
			FROM notes WHERE bear_id = ?`, *note.BearID,
		).Scan(&existingID, &existingSyncStatus, &existingModifiedAt,
			&existingTitle, &existingBody,
			&pendingBearTitle, &pendingBearBody,
			&expectedBearModifiedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup note by bear_id: %w", err)
		}
	}

	if existingID != "" {
		var pbt, pbb *string
		if pendingBearTitle.Valid {
			pbt = &pendingBearTitle.String
		}
		if pendingBearBody.Valid {
			pbb = &pendingBearBody.String
		}
		var ebma *string
		if expectedBearModifiedAt.Valid {
			ebma = &expectedBearModifiedAt.String
		}

		return updateExistingNote(ctx, tx, note, existingID, existingSyncStatus, existingModifiedAt,
			existingTitle, existingBody, pbt, pbb, ebma)
	}

	return insertNewNote(ctx, tx, note)
}

// updateExistingNote updates a note during a Bear delta push.
// For pending_to_bear notes, title/body are deliberately excluded from the UPDATE — the hub record
// preserves the consumer's version. Field-level conflict detection uses pending_bear_title/body
// (Bear's snapshot at enqueue time) to compare against the incoming Bear delta and the consumer's
// changes, so conflicts only fire when both sides modified the same content field.
func updateExistingNote(
	ctx context.Context, tx *sql.Tx, note *models.Note,
	existingID, existingSyncStatus, existingModifiedAt string,
	hubTitle, hubBody string, pendingBearTitle, pendingBearBody *string,
	expectedBearModifiedAt *string,
) error {
	if existingSyncStatus == syncStatusPendingToBear {
		newSyncStatus := syncStatusPendingToBear
		var clearExpectedBearModifiedAt bool

		// Echo detection: if the incoming modified_at matches expected_bear_modified_at,
		// this delta push is an echo of our own write — skip conflict detection.
		if expectedBearModifiedAt != nil && note.ModifiedAt == *expectedBearModifiedAt {
			clearExpectedBearModifiedAt = true
		} else if note.ModifiedAt != "" && existingModifiedAt != "" && note.ModifiedAt != existingModifiedAt {
			newSyncStatus = detectContentConflict(note, hubTitle, hubBody, pendingBearTitle, pendingBearBody)
		}

		var ebmaVal any
		if clearExpectedBearModifiedAt {
			ebmaVal = nil
		} else {
			ebmaVal = expectedBearModifiedAt
		}

		query := `UPDATE notes SET
			bear_id = ?, subtitle = ?,
			archived = ?, encrypted = ?, has_files = ?, has_images = ?,
			has_source_code = ?, locked = ?, pinned = ?, shown_in_today = ?,
			trashed = ?, permanently_deleted = ?, skip_sync = ?,
			todo_completed = ?, todo_incompleted = ?, version = ?,
			created_at = ?, modified_at = ?, archived_at = ?, encrypted_at = ?,
			locked_at = ?, pinned_at = ?, trashed_at = ?, order_date = ?,
			conflict_id_date = ?, last_editing_device = ?, conflict_id = ?,
			encryption_id = ?, encrypted_data = ?, bear_raw = ?,
			sync_status = ?, expected_bear_modified_at = ?
		WHERE id = ?`

		if _, err := tx.ExecContext(ctx, query,
			note.BearID, note.Subtitle,
			note.Archived, note.Encrypted, note.HasFiles, note.HasImages,
			note.HasSourceCode, note.Locked, note.Pinned, note.ShownInToday,
			note.Trashed, note.PermanentlyDeleted, note.SkipSync,
			note.TodoCompleted, note.TodoIncompleted, note.Version,
			note.CreatedAt, note.ModifiedAt, note.ArchivedAt, note.EncryptedAt,
			note.LockedAt, note.PinnedAt, note.TrashedAt, note.OrderDate,
			note.ConflictIDDate, note.LastEditingDevice, note.ConflictID,
			note.EncryptionID, note.EncryptedData, note.BearRaw,
			newSyncStatus, ebmaVal,
			existingID,
		); err != nil {
			return fmt.Errorf("update pending_to_bear note: %w", err)
		}

		return nil
	}

	note.ID = existingID

	// Preserve conflict status: a Bear delta push must not silently clear a conflict
	// that has been set but not yet processed by the bridge.
	if existingSyncStatus == syncStatusConflict {
		note.SyncStatus = syncStatusConflict
	}

	query := `UPDATE notes SET
		bear_id = ?, title = ?, subtitle = ?, body = ?,
		archived = ?, encrypted = ?, has_files = ?, has_images = ?,
		has_source_code = ?, locked = ?, pinned = ?, shown_in_today = ?,
		trashed = ?, permanently_deleted = ?, skip_sync = ?,
		todo_completed = ?, todo_incompleted = ?, version = ?,
		created_at = ?, modified_at = ?, archived_at = ?, encrypted_at = ?,
		locked_at = ?, pinned_at = ?, trashed_at = ?, order_date = ?,
		conflict_id_date = ?, last_editing_device = ?, conflict_id = ?,
		encryption_id = ?, encrypted_data = ?, sync_status = ?,
		hub_modified_at = ?, bear_raw = ?,
		pending_bear_title = ?, pending_bear_body = ?,
		expected_bear_modified_at = ?
	WHERE id = ?`

	vals := noteValues(note)
	vals = append(vals[1:], vals[0])

	if _, err := tx.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update note: %w", err)
	}

	return nil
}

// detectContentConflict determines whether a Bear delta push conflicts with pending consumer changes.
// If pending_bear fields are nil (create flow), falls back to timestamp-based conflict.
// Otherwise, conflict fires only if Bear changed a content field that the consumer also changed.
func detectContentConflict(
	bearDelta *models.Note, hubTitle, hubBody string, pendingBearTitle, pendingBearBody *string,
) string {
	if pendingBearTitle == nil || pendingBearBody == nil {
		return syncStatusConflict
	}

	bearTitleChanged := bearDelta.Title != *pendingBearTitle
	bearBodyChanged := bearDelta.Body != *pendingBearBody
	consumerChangedTitle := hubTitle != *pendingBearTitle
	consumerChangedBody := hubBody != *pendingBearBody

	if (bearTitleChanged && consumerChangedTitle) || (bearBodyChanged && consumerChangedBody) {
		return syncStatusConflict
	}

	return syncStatusPendingToBear
}

func insertNewNote(ctx context.Context, tx *sql.Tx, note *models.Note) error {
	if note.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("generate id: %w", err)
		}

		note.ID = id
	}

	query := `INSERT INTO notes (
		id, bear_id, title, subtitle, body,
		archived, encrypted, has_files, has_images, has_source_code,
		locked, pinned, shown_in_today, trashed, permanently_deleted,
		skip_sync, todo_completed, todo_incompleted, version,
		created_at, modified_at, archived_at, encrypted_at, locked_at,
		pinned_at, trashed_at, order_date, conflict_id_date,
		last_editing_device, conflict_id, encryption_id, encrypted_data,
		sync_status, hub_modified_at, bear_raw,
		pending_bear_title, pending_bear_body,
		expected_bear_modified_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?
	)`

	if _, err := tx.ExecContext(ctx, query, noteValues(note)...); err != nil {
		return fmt.Errorf("insert note: %w", err)
	}

	return nil
}

//nolint:dupl // upsert pattern is similar but SQL schemas differ
func upsertTag(ctx context.Context, tx *sql.Tx, tag *models.Tag) error {
	var existingID string

	if tag.BearID != nil && *tag.BearID != "" {
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM tags WHERE bear_id = ?", *tag.BearID,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup tag by bear_id: %w", err)
		}
	}

	if existingID != "" {
		tag.ID = existingID

		query := `UPDATE tags SET
			bear_id = ?, title = ?, pinned = ?, is_root = ?,
			hide_subtags_notes = ?, sorting = ?, sorting_direction = ?,
			encrypted = ?, version = ?, modified_at = ?, pinned_at = ?,
			pinned_notes_at = ?, encrypted_at = ?, hide_subtags_notes_at = ?,
			sorting_at = ?, sorting_direction_at = ?, tag_con_date = ?,
			tag_con = ?, bear_raw = ?
		WHERE id = ?`

		vals := tagValues(tag)
		vals = append(vals[1:], vals[0])

		if _, err := tx.ExecContext(ctx, query, vals...); err != nil {
			return fmt.Errorf("update tag: %w", err)
		}

		return nil
	}

	if tag.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("generate id: %w", err)
		}

		tag.ID = id
	}

	query := `INSERT INTO tags (
		id, bear_id, title, pinned, is_root, hide_subtags_notes,
		sorting, sorting_direction, encrypted, version,
		modified_at, pinned_at, pinned_notes_at, encrypted_at,
		hide_subtags_notes_at, sorting_at, sorting_direction_at,
		tag_con_date, tag_con, bear_raw
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := tx.ExecContext(ctx, query, tagValues(tag)...); err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}

	return nil
}

//nolint:dupl // upsert pattern is similar but SQL schemas differ
func upsertAttachment(ctx context.Context, tx *sql.Tx, a *models.Attachment) error {
	// Resolve note_id from bear_id to hub UUID (the mapper produces Bear UUIDs).
	if a.NoteID != "" {
		var hubNoteID string
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM notes WHERE bear_id = ? OR id = ?", a.NoteID, a.NoteID,
		).Scan(&hubNoteID)
		if err == nil {
			a.NoteID = hubNoteID
		}
		// If note not found, keep the original ID (may be a hub UUID already or note not yet synced).
	}

	var existingID string

	if a.BearID != nil && *a.BearID != "" {
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE bear_id = ?", *a.BearID,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup attachment by bear_id: %w", err)
		}
	}

	if existingID != "" {
		a.ID = existingID

		query := `UPDATE attachments SET
			bear_id = ?, note_id = ?, type = ?, filename = ?,
			normalized_extension = ?, file_size = ?, file_index = ?,
			width = ?, height = ?, animated = ?, duration = ?,
			width1 = ?, height1 = ?, downloaded = ?, encrypted = ?,
			permanently_deleted = ?, skip_sync = ?, unused = ?,
			uploaded = ?, version = ?, created_at = ?, modified_at = ?,
			inserted_at = ?, encrypted_at = ?, unused_at = ?,
			uploaded_at = ?, search_text_at = ?, last_editing_device = ?,
			encryption_id = ?, search_text = ?, encrypted_data = ?,
			file_path = ?, bear_raw = ?
		WHERE id = ?`

		vals := attachmentValues(a)
		vals = append(vals[1:], vals[0])

		if _, err := tx.ExecContext(ctx, query, vals...); err != nil {
			return fmt.Errorf("update attachment: %w", err)
		}

		return nil
	}

	if a.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("generate id: %w", err)
		}

		a.ID = id
	}

	query := `INSERT INTO attachments (
		id, bear_id, note_id, type, filename, normalized_extension,
		file_size, file_index, width, height, animated, duration,
		width1, height1, downloaded, encrypted, permanently_deleted,
		skip_sync, unused, uploaded, version, created_at, modified_at,
		inserted_at, encrypted_at, unused_at, uploaded_at, search_text_at,
		last_editing_device, encryption_id, search_text, encrypted_data,
		file_path, bear_raw
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
	)`

	if _, err := tx.ExecContext(ctx, query, attachmentValues(a)...); err != nil {
		return fmt.Errorf("insert attachment: %w", err)
	}

	return nil
}

//nolint:dupl // upsert pattern is similar but SQL schemas differ
func upsertBacklink(ctx context.Context, tx *sql.Tx, b *models.Backlink) error {
	// Resolve linked_by_id and linking_to_id from bear_id to hub UUID.
	b.LinkedByID = resolveNoteID(ctx, tx, b.LinkedByID)
	b.LinkingToID = resolveNoteID(ctx, tx, b.LinkingToID)

	// Skip backlinks where either side doesn't exist on the hub.
	// This happens when a note was deleted from Bear (LEFT JOIN → empty string)
	// or when a note exists in Bear but hasn't been synced to the hub yet.
	if b.LinkedByID == "" || !noteExistsInTx(ctx, tx, b.LinkedByID) {
		return nil
	}
	if b.LinkingToID == "" || !noteExistsInTx(ctx, tx, b.LinkingToID) {
		return nil
	}

	var existingID string

	if b.BearID != nil && *b.BearID != "" {
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM backlinks WHERE bear_id = ?", *b.BearID,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup backlink by bear_id: %w", err)
		}
	}

	if existingID != "" {
		b.ID = existingID

		query := `UPDATE backlinks SET
			bear_id = ?, linked_by_id = ?, linking_to_id = ?,
			title = ?, location = ?, version = ?,
			modified_at = ?, bear_raw = ?
		WHERE id = ?`

		vals := backlinkValues(b)
		vals = append(vals[1:], vals[0])

		if _, err := tx.ExecContext(ctx, query, vals...); err != nil {
			return fmt.Errorf("update backlink: %w", err)
		}

		return nil
	}

	if b.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("generate id: %w", err)
		}

		b.ID = id
	}

	query := `INSERT INTO backlinks (
		id, bear_id, linked_by_id, linking_to_id,
		title, location, version, modified_at, bear_raw
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := tx.ExecContext(ctx, query, backlinkValues(b)...); err != nil {
		return fmt.Errorf("insert backlink: %w", err)
	}

	return nil
}

func replaceNoteTags(
	ctx context.Context, tx *sql.Tx, table string,
	notes []models.Note, pairs []models.NoteTagPair,
) error {
	if len(notes) == 0 && len(pairs) == 0 {
		return nil
	}

	noteHubIDs := make(map[string]string)

	// Resolve note IDs from the Notes slice (if present in same push).
	for i := range notes {
		if notes[i].BearID != nil && *notes[i].BearID != "" {
			var hubID string

			err := tx.QueryRowContext(ctx,
				"SELECT id FROM notes WHERE bear_id = ?", *notes[i].BearID,
			).Scan(&hubID)
			if err == nil {
				noteHubIDs[*notes[i].BearID] = hubID
			}
		}

		if notes[i].ID != "" {
			noteHubIDs[notes[i].ID] = notes[i].ID
		}
	}

	// Also resolve note IDs referenced in pairs (notes may have been pushed in a separate request).
	for _, pair := range pairs {
		if _, ok := noteHubIDs[pair.NoteID]; !ok {
			var hubID string

			err := tx.QueryRowContext(ctx,
				"SELECT id FROM notes WHERE bear_id = ? OR id = ?", pair.NoteID, pair.NoteID,
			).Scan(&hubID)
			if err == nil {
				noteHubIDs[pair.NoteID] = hubID
			}
		}
	}

	if len(noteHubIDs) == 0 {
		return nil
	}

	hubIDSet := make(map[string]bool)
	for _, id := range noteHubIDs {
		hubIDSet[id] = true
	}

	hubIDs := make([]string, 0, len(hubIDSet))
	for id := range hubIDSet {
		hubIDs = append(hubIDs, id)
	}

	placeholders := make([]string, len(hubIDs))
	deleteArgs := make([]any, len(hubIDs))

	for i, id := range hubIDs {
		placeholders[i] = "?"
		deleteArgs[i] = id
	}

	//nolint:gosec // table is a hardcoded constant ("note_tags" or "pinned_note_tags")
	deleteQuery := "DELETE FROM " + table +
		" WHERE note_id IN (" + strings.Join(placeholders, ",") + ")"

	if _, err := tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return fmt.Errorf("delete %s: %w", table, err)
	}

	return insertNoteTagPairs(ctx, tx, table, pairs, noteHubIDs)
}

func insertNoteTagPairs(
	ctx context.Context, tx *sql.Tx, table string,
	pairs []models.NoteTagPair, noteHubIDs map[string]string,
) error {
	for _, pair := range pairs {
		// Skip sentinel pairs (empty TagID) used to signal "clear all tags for this note".
		// The note was already resolved and its tags deleted above; nothing to insert.
		if pair.TagID == "" {
			continue
		}

		resolved, ok := noteHubIDs[pair.NoteID]
		if !ok {
			slog.Warn("skip note-tag pair: note not found",
				"note_id", pair.NoteID, "table", table)

			continue
		}

		var resolvedTagID string

		err := tx.QueryRowContext(ctx,
			"SELECT id FROM tags WHERE bear_id = ? OR id = ?", pair.TagID, pair.TagID,
		).Scan(&resolvedTagID)
		if err != nil {
			slog.Warn("skip note-tag pair: tag not found",
				"tag_id", pair.TagID, "table", table)

			continue
		}

		//nolint:gosec // table is a hardcoded constant
		insertQuery := "INSERT OR IGNORE INTO " + table +
			" (note_id, tag_id) VALUES (?, ?)"

		if _, err := tx.ExecContext(ctx, insertQuery, resolved, resolvedTagID); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}

	return nil
}

// resolveNoteID resolves a Bear UUID or hub UUID to the hub note ID.
func noteExistsInTx(ctx context.Context, tx *sql.Tx, id string) bool {
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM notes WHERE id = ?", id).Scan(&exists)
	return err == nil
}

func resolveNoteID(ctx context.Context, tx *sql.Tx, id string) string {
	if id == "" {
		return id
	}

	var hubID string

	err := tx.QueryRowContext(ctx, "SELECT id FROM notes WHERE bear_id = ? OR id = ?", id, id).Scan(&hubID)
	if err == nil {
		return hubID
	}

	return id
}

// failQueueItemsForDeletedNotes marks pending/processing write_queue items as failed
// for any notes that are about to be deleted. This prevents orphaned queue items
// whose note_id no longer references any row in the notes table.
func failQueueItemsForDeletedNotes(ctx context.Context, tx *sql.Tx, bearIDs []string) error {
	if len(bearIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(bearIDs))
	args := make([]any, len(bearIDs))

	for i, id := range bearIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Resolve bear_ids to hub note IDs, then fail their queue items.
	//nolint:gosec // placeholders are parameterized "?" values, not user input
	query := "UPDATE write_queue SET status = 'failed', error = 'note deleted from Bear'" +
		" WHERE status IN ('pending', 'processing')" +
		" AND note_id IN (SELECT id FROM notes WHERE bear_id IN (" + strings.Join(placeholders, ",") + "))"

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("fail queue items: %w", err)
	}

	return nil
}

func deleteByBearIDs(ctx context.Context, tx *sql.Tx, table string, bearIDs []string) error {
	if len(bearIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(bearIDs))
	args := make([]any, len(bearIDs))

	for i, id := range bearIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	//nolint:gosec // table is a hardcoded constant ("notes", "tags", "attachments", "backlinks")
	query := "DELETE FROM " + table +
		" WHERE bear_id IN (" + strings.Join(placeholders, ",") + ")"

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}

	return nil
}

// --- Write Queue ---

func (s *SQLiteStore) GetQueueItem(ctx context.Context, id int64) (*models.WriteQueueItem, error) {
	item, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
		"SELECT "+writeQueueColumns()+" FROM write_queue WHERE id = ?", id,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get queue item: %w", err)
	}

	return &item, nil
}

func (s *SQLiteStore) GetQueueItemByIdempotencyKey(
	ctx context.Context, key, consumerID string,
) (*models.WriteQueueItem, error) {
	item, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
		"SELECT "+writeQueueColumns()+" FROM write_queue WHERE idempotency_key = ? AND consumer_id = ?",
		key, consumerID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get queue item by idempotency key: %w", err)
	}

	return &item, nil
}

func (s *SQLiteStore) EnqueueWrite(
	ctx context.Context,
	idempotencyKey, action, noteID, payload, consumerID string,
) (*models.WriteQueueItem, error) {
	existing, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
		"SELECT "+writeQueueColumns()+" FROM write_queue WHERE idempotency_key = ? AND consumer_id = ?",
		idempotencyKey, consumerID,
	))
	if err == nil {
		// If the existing item failed, reset it to pending so it can be retried.
		// The WHERE clause includes status = 'failed' to prevent a race where two concurrent
		// retries both observe 'failed' but only one actually resets the item.
		if existing.Status == "failed" {
			res, err := s.db.ExecContext(ctx,
				`UPDATE write_queue SET status = 'pending', action = ?, note_id = ?, payload = ?,
				processing_by = NULL, lease_until = NULL, applied_at = NULL, error = NULL
				WHERE id = ? AND status = 'failed'`,
				action, noteID, payload, existing.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("reset failed queue item: %w", err)
			}

			rowsAffected, _ := res.RowsAffected() //nolint:errcheck // SQLite always supports RowsAffected
			if rowsAffected == 0 {
				// Another goroutine already reset this item; re-read and return as idempotent hit.
				item, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
					"SELECT "+writeQueueColumns()+" FROM write_queue WHERE id = ?", existing.ID,
				))
				if err != nil {
					return nil, fmt.Errorf("read concurrently reset queue item: %w", err)
				}

				return &item, nil
			}

			item, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
				"SELECT "+writeQueueColumns()+" FROM write_queue WHERE id = ?", existing.ID,
			))
			if err != nil {
				return nil, fmt.Errorf("read reset queue item: %w", err)
			}

			return &item, nil
		}

		return &existing, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check idempotency: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO write_queue (idempotency_key, action, note_id, payload, consumer_id)
		VALUES (?, ?, ?, ?, ?)`,
		idempotencyKey, action, noteID, payload, consumerID,
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue write: %w", err)
	}

	id, _ := result.LastInsertId() //nolint:errcheck // SQLite always supports LastInsertId

	item, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
		"SELECT "+writeQueueColumns()+" FROM write_queue WHERE id = ?", id,
	))
	if err != nil {
		return nil, fmt.Errorf("read enqueued item: %w", err)
	}

	return &item, nil
}

func (s *SQLiteStore) LeaseQueueItems(
	ctx context.Context, processingBy string, leaseDuration time.Duration,
) ([]models.WriteQueueItem, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	leaseUntil := time.Now().UTC().Add(leaseDuration).Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin lease tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if _, err = tx.ExecContext(ctx,
		"UPDATE write_queue SET status = 'pending', processing_by = NULL, lease_until = NULL "+
			"WHERE status = 'processing' AND lease_until < ?",
		now,
	); err != nil {
		return nil, fmt.Errorf("expire stale leases: %w", err)
	}

	//nolint:gosec // columns are internal constants
	rows, err := tx.QueryContext(ctx,
		"SELECT "+prefixedWriteQueueColumns("wq")+", COALESCE(n.sync_status, '')"+
			" FROM write_queue wq LEFT JOIN notes n ON n.id = wq.note_id"+
			" WHERE wq.status = 'pending' ORDER BY wq.id LIMIT 100",
	)
	if err != nil {
		return nil, fmt.Errorf("select pending items: %w", err)
	}

	var items []models.WriteQueueItem

	for rows.Next() {
		item, scanErr := scanWriteQueueWithSyncStatus(rows)
		if scanErr != nil {
			_ = rows.Close()

			return nil, fmt.Errorf("scan queue item: %w", scanErr)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()

		return nil, fmt.Errorf("iterate queue items: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close queue rows: %w", err)
	}

	if len(items) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty lease: %w", err)
		}

		return nil, nil
	}

	ids := make([]string, len(items))
	args := make([]any, 0, len(items)+2)
	args = append(args, processingBy, leaseUntil)

	for i := range items {
		ids[i] = "?"
		args = append(args, items[i].ID)
	}

	//nolint:gosec // ids are parameterized placeholders, not user input
	leaseQuery := "UPDATE write_queue SET status = 'processing', " +
		"processing_by = ?, lease_until = ? WHERE id IN (" +
		strings.Join(ids, ",") + ")"

	if _, err = tx.ExecContext(ctx, leaseQuery, args...); err != nil {
		return nil, fmt.Errorf("update lease: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit lease: %w", err)
	}

	for i := range items {
		items[i].Status = "processing"
		items[i].ProcessingBy = processingBy
		items[i].LeaseUntil = leaseUntil
	}

	return items, nil
}

// AckQueueItems acknowledges leased write-queue items, transitioning them to terminal states.
//
// Safety invariant: the single bridge processes queue items sequentially (lease → apply → ack)
// within one goroutine. Combined with the 60 s HTTP client timeout (well within the 5-minute
// lease window), stale acks from a previous attempt cannot arrive after a re-lease because the
// underlying TCP connection is closed on timeout. No lease-version token is therefore required.
func (s *SQLiteStore) AckQueueItems(ctx context.Context, items []models.SyncAckItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ack tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	now := time.Now().UTC().Format(time.RFC3339)

	for i := range items {
		if err := ackSingleItem(ctx, tx, &items[i], now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ack: %w", err)
	}

	return nil
}

func ackSingleItem(ctx context.Context, tx *sql.Tx, item *models.SyncAckItem, now string) error {
	var currentStatus string

	err := tx.QueryRowContext(ctx,
		"SELECT status FROM write_queue WHERE id = ?",
		item.QueueID,
	).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("check ack status: %w", err)
	}

	// Only apply acks to items currently being processed. Skip if:
	// - "pending": item was reset by EnqueueWrite retry; this ack is stale
	// - "applied"/"failed": already terminal
	if currentStatus != "processing" {
		return nil
	}

	if item.Status == "applied" {
		return ackApplied(ctx, tx, item, now)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE write_queue SET status = 'failed', error = ? WHERE id = ?",
		item.Error, item.QueueID,
	); err != nil {
		return fmt.Errorf("ack failed: %w", err)
	}

	return nil
}

func ackApplied(ctx context.Context, tx *sql.Tx, item *models.SyncAckItem, now string) error {
	if _, err := tx.ExecContext(ctx,
		"UPDATE write_queue SET status = 'applied', applied_at = ?, error = NULL "+
			"WHERE id = ?",
		now, item.QueueID,
	); err != nil {
		return fmt.Errorf("ack applied: %w", err)
	}

	var noteID sql.NullString

	err := tx.QueryRowContext(ctx,
		"SELECT note_id FROM write_queue WHERE id = ?",
		item.QueueID,
	).Scan(&noteID)
	if err == nil && noteID.Valid && noteID.String != "" {
		return ackUpdateNoteStatus(ctx, tx, item, noteID.String)
	}

	return nil
}

func ackUpdateNoteStatus(ctx context.Context, tx *sql.Tx, item *models.SyncAckItem, noteID string) error {
	// Check if other pending/processing queue items exist for this note.
	// If so, keep sync_status as pending_to_bear to protect hub content from Bear delta overwrites.
	var otherPending int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM write_queue WHERE note_id = ? AND id != ? AND status IN ('pending', 'processing')",
		noteID, item.QueueID,
	).Scan(&otherPending); err != nil {
		return fmt.Errorf("check other pending queue items: %w", err)
	}

	switch {
	case item.ConflictResolved:
		// Bridge handled a conflict item: clear conflict status only if still in conflict
		// and no other consumers have pending writes.
		if otherPending == 0 {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET sync_status = 'synced', "+
					"pending_bear_title = NULL, pending_bear_body = NULL, expected_bear_modified_at = NULL "+
					"WHERE id = ? AND sync_status = 'conflict'",
				noteID,
			); err != nil {
				return fmt.Errorf("clear conflict status on ack: %w", err)
			}
		}
	case item.BearID != "":
		if otherPending > 0 {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET bear_id = ?, sync_status = ?, expected_bear_modified_at = ? "+
					"WHERE id = ? AND sync_status != 'conflict'",
				item.BearID, syncStatusPendingToBear, toNullString(item.BearModifiedAt), noteID,
			); err != nil {
				return fmt.Errorf("set bear_id on ack: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET bear_id = ?, sync_status = 'synced', "+
					"pending_bear_title = NULL, pending_bear_body = NULL, expected_bear_modified_at = NULL "+
					"WHERE id = ? AND sync_status != 'conflict'",
				item.BearID, noteID,
			); err != nil {
				return fmt.Errorf("set bear_id on ack: %w", err)
			}
		}
	default:
		if otherPending == 0 {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET sync_status = 'synced', "+
					"pending_bear_title = NULL, pending_bear_body = NULL, expected_bear_modified_at = NULL "+
					"WHERE id = ? AND sync_status != 'conflict'",
				noteID,
			); err != nil {
				return fmt.Errorf("reset sync_status on ack: %w", err)
			}
		} else if item.BearModifiedAt != "" {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET expected_bear_modified_at = ? WHERE id = ? AND sync_status != 'conflict'",
				item.BearModifiedAt, noteID,
			); err != nil {
				return fmt.Errorf("set expected_bear_modified_at on ack: %w", err)
			}
		}
	}

	return nil
}

func (s *SQLiteStore) PendingQueueCount(ctx context.Context) (int, error) {
	var count int

	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM write_queue WHERE status = 'pending' OR status = 'processing'",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending queue: %w", err)
	}

	return count, nil
}

// --- Conflicts ---

func (s *SQLiteStore) CountConflicts(ctx context.Context) (int, error) {
	var count int

	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notes WHERE sync_status = 'conflict'",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count conflicts: %w", err)
	}

	return count, nil
}

func (s *SQLiteStore) ListConflictNoteIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM notes WHERE sync_status = 'conflict' ORDER BY modified_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("list conflict note IDs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var ids []string

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conflict note ID: %w", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conflict note IDs: %w", err)
	}

	return ids, nil
}

// --- Sync Meta ---

func (s *SQLiteStore) GetSyncMeta(ctx context.Context, key string) (string, error) {
	var value string

	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM sync_meta WHERE key = ?", key,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("get sync meta: %w", err)
	}

	return value, nil
}

func (s *SQLiteStore) SetSyncMeta(ctx context.Context, key, value string) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO sync_meta (key, value) VALUES (?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	); err != nil {
		return fmt.Errorf("set sync meta: %w", err)
	}

	return nil
}

// --- Helpers ---

func (s *SQLiteStore) loadNoteTags(ctx context.Context, noteID string) ([]models.Tag, error) {
	//nolint:gosec // columns are internal constants
	query := "SELECT " + tagColumnsWithPrefix("t.") +
		" FROM tags t JOIN note_tags nt ON t.id = nt.tag_id" +
		" WHERE nt.note_id = ? ORDER BY t.title"

	rows, err := s.db.QueryContext(ctx, query, noteID)
	if err != nil {
		return nil, fmt.Errorf("query note tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	var tags []models.Tag

	for rows.Next() {
		t, scanErr := scanTag(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan note tag: %w", scanErr)
		}

		tags = append(tags, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate note tags: %w", err)
	}

	return tags, nil
}

// toNullString converts an empty string to nil (SQL NULL) and a non-empty string to *string.
func toNullString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hub id: %w", err)
	}

	return hex.EncodeToString(b), nil
}
