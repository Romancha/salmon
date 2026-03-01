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

	"github.com/romancha/bear-sync/internal/models"

	_ "modernc.org/sqlite"
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
    bear_raw            TEXT
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
    idempotency_key TEXT UNIQUE NOT NULL,
    action          TEXT NOT NULL,
    note_id         TEXT,
    payload         TEXT NOT NULL,
    created_at      TEXT DEFAULT (datetime('now')),
    status          TEXT DEFAULT 'pending',
    processing_by   TEXT,
    lease_until     TEXT,
    applied_at      TEXT,
    error           TEXT
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

	return nil
}

// --- Notes ---

//nolint:gocritic // intentional value receiver for simple filter struct
func (s *SQLiteStore) ListNotes(ctx context.Context, filter NoteFilter) ([]models.Note, error) {
	var where []string
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
		sync_status, hub_modified_at, bear_raw
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?
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
		hub_modified_at = ?, bear_raw = ?
	WHERE id = ?`

	vals := noteValues(note)
	vals = append(vals[1:], vals[0])

	if _, err := s.db.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update note: %w", err)
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

	where = append(where, "notes_fts MATCH ?")
	args = append(args, query)

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

	if note.BearID != nil && *note.BearID != "" {
		err := tx.QueryRowContext(ctx,
			"SELECT id, sync_status, COALESCE(modified_at, '') FROM notes WHERE bear_id = ?", *note.BearID,
		).Scan(&existingID, &existingSyncStatus, &existingModifiedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup note by bear_id: %w", err)
		}
	}

	if existingID != "" {
		return updateExistingNote(ctx, tx, note, existingID, existingSyncStatus, existingModifiedAt)
	}

	return insertNewNote(ctx, tx, note)
}

func updateExistingNote(
	ctx context.Context, tx *sql.Tx, note *models.Note,
	existingID, existingSyncStatus, existingModifiedAt string,
) error {
	if existingSyncStatus == "pending_to_bear" {
		// Conflict detection: if Bear's modified_at changed since our last sync,
		// the user edited the note while openclaw had pending changes.
		newSyncStatus := "pending_to_bear"
		if note.ModifiedAt != "" && existingModifiedAt != "" && note.ModifiedAt != existingModifiedAt {
			newSyncStatus = "conflict"
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
			sync_status = ?
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
			newSyncStatus,
			existingID,
		); err != nil {
			return fmt.Errorf("update pending_to_bear note: %w", err)
		}

		return nil
	}

	note.ID = existingID

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
		hub_modified_at = ?, bear_raw = ?
	WHERE id = ?`

	vals := noteValues(note)
	vals = append(vals[1:], vals[0])

	if _, err := tx.ExecContext(ctx, query, vals...); err != nil {
		return fmt.Errorf("update note: %w", err)
	}

	return nil
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
		sync_status, hub_modified_at, bear_raw
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?
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
	if len(notes) == 0 {
		return nil
	}

	noteHubIDs := make(map[string]string)

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

	for _, pair := range pairs {
		noteID := pair.NoteID
		tagID := pair.TagID

		if resolved, ok := noteHubIDs[noteID]; ok {
			noteID = resolved
		}

		var resolvedTagID string

		err := tx.QueryRowContext(ctx,
			"SELECT id FROM tags WHERE bear_id = ? OR id = ?", tagID, tagID,
		).Scan(&resolvedTagID)
		if err != nil {
			slog.Warn("skip note-tag pair: tag not found",
				"tag_id", tagID, "table", table)

			continue
		}

		//nolint:gosec // table is a hardcoded constant
		insertQuery := "INSERT OR IGNORE INTO " + table +
			" (note_id, tag_id) VALUES (?, ?)"

		if _, err := tx.ExecContext(ctx, insertQuery, noteID, resolvedTagID); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
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

func (s *SQLiteStore) EnqueueWrite(
	ctx context.Context,
	idempotencyKey, action, noteID, payload string,
) (*models.WriteQueueItem, error) {
	existing, err := scanWriteQueueRow(s.db.QueryRowContext(ctx,
		"SELECT "+writeQueueColumns()+" FROM write_queue WHERE idempotency_key = ?",
		idempotencyKey,
	))
	if err == nil {
		return &existing, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check idempotency: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO write_queue (idempotency_key, action, note_id, payload)
		VALUES (?, ?, ?, ?)`,
		idempotencyKey, action, noteID, payload,
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
			" WHERE wq.status = 'pending' ORDER BY wq.id",
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

	_ = rows.Close()

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue items: %w", err)
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

func (s *SQLiteStore) AckQueueItems(ctx context.Context, items []models.SyncAckItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ack tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	now := time.Now().UTC().Format(time.RFC3339)

	for _, item := range items {
		if err := ackSingleItem(ctx, tx, item, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ack: %w", err)
	}

	return nil
}

func ackSingleItem(ctx context.Context, tx *sql.Tx, item models.SyncAckItem, now string) error {
	var currentStatus string

	err := tx.QueryRowContext(ctx,
		"SELECT status FROM write_queue WHERE idempotency_key = ?",
		item.IdempotencyKey,
	).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("check ack status: %w", err)
	}

	if currentStatus == "applied" || currentStatus == "failed" {
		return nil
	}

	if item.Status == "applied" {
		return ackApplied(ctx, tx, item, now)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE write_queue SET status = 'failed', error = ? WHERE idempotency_key = ?",
		item.Error, item.IdempotencyKey,
	); err != nil {
		return fmt.Errorf("ack failed: %w", err)
	}

	return nil
}

func ackApplied(ctx context.Context, tx *sql.Tx, item models.SyncAckItem, now string) error {
	if _, err := tx.ExecContext(ctx,
		"UPDATE write_queue SET status = 'applied', applied_at = ?, error = NULL "+
			"WHERE idempotency_key = ?",
		now, item.IdempotencyKey,
	); err != nil {
		return fmt.Errorf("ack applied: %w", err)
	}

	if item.BearID != "" {
		var noteID sql.NullString

		err := tx.QueryRowContext(ctx,
			"SELECT note_id FROM write_queue WHERE idempotency_key = ?",
			item.IdempotencyKey,
		).Scan(&noteID)
		if err == nil && noteID.Valid && noteID.String != "" {
			if _, err := tx.ExecContext(ctx,
				"UPDATE notes SET bear_id = ?, sync_status = 'synced' WHERE id = ?",
				item.BearID, noteID.String,
			); err != nil {
				return fmt.Errorf("set bear_id on ack: %w", err)
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

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hub id: %w", err)
	}

	return hex.EncodeToString(b), nil
}
