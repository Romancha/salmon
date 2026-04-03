package beardb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/romancha/salmon/internal/mapper"

	_ "modernc.org/sqlite" // SQLite driver
)

// DefaultBearDBPath is the default path to Bear's SQLite database on macOS.
const DefaultBearDBPath = "~/Library/Group Containers/9K33E3U3T4.net.shinyfrog.bear/Application Data/database.sqlite"

// SQLiteBearDB implements BearDB by reading Bear's SQLite database in read-only mode.
type SQLiteBearDB struct {
	db *sql.DB
}

// New opens Bear's SQLite database in read-only mode.
func New(dbPath string) (*SQLiteBearDB, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open bear db: %w", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		db.Close() //nolint:errcheck,gosec // best-effort close on init failure
		return nil, fmt.Errorf("ping bear db: %w", err)
	}

	return &SQLiteBearDB{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteBearDB) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close bear db: %w", err)
	}

	return nil
}

// noteColumns is the SELECT column list for ZSFNOTE.
const noteColumns = `Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT,
	ZARCHIVED, ZENCRYPTED, ZHASFILES, ZHASIMAGES, ZHASSOURCECODE,
	ZLOCKED, ZPINNED, ZSHOWNINTODAYWIDGET, ZTRASHED, ZPERMANENTLYDELETED,
	ZSKIPSYNC, ZTODOCOMPLETED, ZTODOINCOMPLETED, ZVERSION,
	ZCREATIONDATE, ZMODIFICATIONDATE, ZARCHIVEDDATE, ZENCRYPTIONDATE,
	ZLOCKEDDATE, ZPINNEDDATE, ZTRASHEDDATE, ZORDERDATE,
	ZCONFLICTUNIQUEIDENTIFIERDATE, ZLASTEDITINGDEVICE,
	ZCONFLICTUNIQUEIDENTIFIER, ZENCRYPTIONUNIQUEIDENTIFIER, ZENCRYPTEDDATA`

// Notes returns notes modified since lastSyncAt (Core Data epoch).
//
//nolint:dupl // Notes and Tags share query pattern but scan different schemas
func (s *SQLiteBearDB) Notes(ctx context.Context, lastSyncAt float64) ([]mapper.BearNoteRow, error) {
	query := "SELECT " + noteColumns + " FROM ZSFNOTE" //nolint:gosec // column list is a constant
	var args []any

	if lastSyncAt > 0 {
		query += " WHERE ZMODIFICATIONDATE >= ?"
		args = append(args, lastSyncAt)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []mapper.BearNoteRow

	for rows.Next() {
		var row mapper.BearNoteRow
		if err := rows.Scan(
			&row.ZPK, &row.ZUNIQUEIDENTIFIER, &row.ZTITLE, &row.ZSUBTITLE, &row.ZTEXT,
			&row.ZARCHIVED, &row.ZENCRYPTED, &row.ZHASFILES, &row.ZHASIMAGES, &row.ZHASSOURCECODE,
			&row.ZLOCKED, &row.ZPINNED, &row.ZSHOWNINTODAYWIDGET, &row.ZTRASHED, &row.ZPERMANENTLYDELETED,
			&row.ZSKIPSYNC, &row.ZTODOCOMPLETED, &row.ZTODOINCOMPLETED, &row.ZVERSION,
			&row.ZCREATIONDATE, &row.ZMODIFICATIONDATE, &row.ZARCHIVEDDATE, &row.ZENCRYPTIONDATE,
			&row.ZLOCKEDDATE, &row.ZPINNEDDATE, &row.ZTRASHEDDATE, &row.ZORDERDATE,
			&row.ZCONFLICTUNIQUEIDENTIFIERDATE, &row.ZLASTEDITINGDEVICE,
			&row.ZCONFLICTUNIQUEIDENTIFIER, &row.ZENCRYPTIONUNIQUEIDENTIFIER, &row.ZENCRYPTEDDATA,
		); err != nil {
			return nil, fmt.Errorf("scan note row: %w", err)
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate note rows: %w", err)
	}

	return result, nil
}

// tagColumns is the SELECT column list for ZSFNOTETAG.
const tagColumns = `Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT,
	ZHIDESUBTAGSNOTES, ZSORTING, ZSORTINGDIRECTION, ZENCRYPTED, ZVERSION,
	ZMODIFICATIONDATE, ZPINNEDDATE, ZPINNEDNOTESDATE, ZENCRYPTEDDATE,
	ZHIDESUBTAGSNOTESDATE, ZSORTINGDATE, ZSORTINGDIRECTIONDATE, ZTAGCONDATE, ZTAGCON`

// Tags returns tags modified since lastSyncAt (Core Data epoch).
//
//nolint:dupl // Tags and Notes share query pattern but scan different schemas
func (s *SQLiteBearDB) Tags(ctx context.Context, lastSyncAt float64) ([]mapper.BearTagRow, error) {
	query := "SELECT " + tagColumns + " FROM ZSFNOTETAG" //nolint:gosec // column list is a constant
	var args []any

	if lastSyncAt > 0 {
		query += " WHERE ZMODIFICATIONDATE >= ?"
		args = append(args, lastSyncAt)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []mapper.BearTagRow

	for rows.Next() {
		var row mapper.BearTagRow
		if err := rows.Scan(
			&row.ZPK, &row.ZUNIQUEIDENTIFIER, &row.ZTITLE, &row.ZPINNED, &row.ZISROOT,
			&row.ZHIDESUBTAGSNOTES, &row.ZSORTING, &row.ZSORTINGDIRECTION, &row.ZENCRYPTED, &row.ZVERSION,
			&row.ZMODIFICATIONDATE, &row.ZPINNEDDATE, &row.ZPINNEDNOTESDATE, &row.ZENCRYPTEDDATE,
			&row.ZHIDESUBTAGSNOTESDATE, &row.ZSORTINGDATE, &row.ZSORTINGDIRECTIONDATE, &row.ZTAGCONDATE,
			&row.ZTAGCON,
		); err != nil {
			return nil, fmt.Errorf("scan tag row: %w", err)
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag rows: %w", err)
	}

	return result, nil
}

// attachmentColumns is the SELECT column list for ZSFNOTEFILE with JOIN to resolve note UUID.
const attachmentColumns = `f.Z_PK, f.Z_ENT, f.ZUNIQUEIDENTIFIER, n.ZUNIQUEIDENTIFIER,
	f.ZFILENAME, f.ZNORMALIZEDFILEEXTENSION, f.ZFILESIZE, f.ZINDEX,
	f.ZWIDTH, f.ZHEIGHT, f.ZANIMATED, f.ZDURATION, f.ZWIDTH1, f.ZHEIGHT1,
	f.ZDOWNLOADED, f.ZENCRYPTED, f.ZPERMANENTLYDELETED, f.ZSKIPSYNC,
	f.ZUNUSED, f.ZUPLOADED, f.ZVERSION,
	f.ZCREATIONDATE, f.ZMODIFICATIONDATE, f.ZINSERTIONDATE, f.ZENCRYPTIONDATE,
	f.ZUNUSEDDATE, f.ZUPLOADEDDATE, f.ZSEARCHTEXTDATE,
	f.ZLASTEDITINGDEVICE, f.ZENCRYPTIONUNIQUEIDENTIFIER, f.ZSEARCHTEXT, f.ZENCRYPTEDDATA`

// Attachments returns attachments modified since lastSyncAt (Core Data epoch).
func (s *SQLiteBearDB) Attachments(ctx context.Context, lastSyncAt float64) ([]mapper.BearAttachmentRow, error) {
	//nolint:gosec // column list is a constant, ZNOTE is Bear's FK column name
	query := "SELECT " + attachmentColumns + " FROM ZSFNOTEFILE f LEFT JOIN ZSFNOTE n ON f.ZNOTE = n.Z_PK"
	var args []any

	if lastSyncAt > 0 {
		query += " WHERE f.ZMODIFICATIONDATE >= ?"
		args = append(args, lastSyncAt)
	}

	return s.queryAttachments(ctx, query, args...)
}

// AttachmentsByUUIDs returns attachments matching the given Bear UUIDs.
func (s *SQLiteBearDB) AttachmentsByUUIDs(ctx context.Context, uuids []string) ([]mapper.BearAttachmentRow, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(uuids))
	args := make([]any, len(uuids))
	for i, u := range uuids {
		placeholders[i] = "?"
		args[i] = u
	}

	//nolint:gosec // column list is a constant, placeholders are generated
	query := "SELECT " + attachmentColumns + " FROM ZSFNOTEFILE f LEFT JOIN ZSFNOTE n ON f.ZNOTE = n.Z_PK" +
		" WHERE f.ZUNIQUEIDENTIFIER IN (" + strings.Join(placeholders, ",") + ")"

	return s.queryAttachments(ctx, query, args...)
}

// queryAttachments executes a query and scans rows into BearAttachmentRow slices.
//
//nolint:dupl // shares scan pattern with Notes/Tags but scans different schema
func (s *SQLiteBearDB) queryAttachments(ctx context.Context, query string, args ...any) ([]mapper.BearAttachmentRow, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []mapper.BearAttachmentRow

	for rows.Next() {
		var row mapper.BearAttachmentRow
		if err := rows.Scan(
			&row.ZPK, &row.ZENT, &row.ZUNIQUEIDENTIFIER, &row.ZNOTE,
			&row.ZFILENAME, &row.ZNORMALIZEDFILEEXTENSION, &row.ZFILESIZE, &row.ZINDEX,
			&row.ZWIDTH, &row.ZHEIGHT, &row.ZANIMATED, &row.ZDURATION, &row.ZWIDTH1, &row.ZHEIGHT1,
			&row.ZDOWNLOADED, &row.ZENCRYPTED, &row.ZPERMANENTLYDELETED, &row.ZSKIPSYNC,
			&row.ZUNUSED, &row.ZUPLOADED, &row.ZVERSION,
			&row.ZCREATIONDATE, &row.ZMODIFICATIONDATE, &row.ZINSERTIONDATE, &row.ZENCRYPTIONDATE,
			&row.ZUNUSEDDATE, &row.ZUPLOADEDDATE, &row.ZSEARCHTEXTDATE,
			&row.ZLASTEDITINGDEVICE, &row.ZENCRYPTIONUNIQUEIDENTIFIER, &row.ZSEARCHTEXT, &row.ZENCRYPTEDDATA,
		); err != nil {
			return nil, fmt.Errorf("scan attachment row: %w", err)
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment rows: %w", err)
	}

	return result, nil
}

// backlinkColumns is the SELECT column list for ZSFNOTEBACKLINK with JOINs to resolve note UUIDs.
const backlinkColumns = `b.Z_PK, b.ZUNIQUEIDENTIFIER, nb.ZUNIQUEIDENTIFIER, nt.ZUNIQUEIDENTIFIER,
	b.ZTITLE, b.ZLOCATION, b.ZVERSION, b.ZMODIFICATIONDATE`

// Backlinks returns backlinks modified since lastSyncAt (Core Data epoch).
func (s *SQLiteBearDB) Backlinks(ctx context.Context, lastSyncAt float64) ([]mapper.BearBacklinkRow, error) {
	//nolint:gosec // column list is a constant
	query := "SELECT " + backlinkColumns +
		" FROM ZSFNOTEBACKLINK b" +
		" LEFT JOIN ZSFNOTE nb ON b.ZLINKEDBY = nb.Z_PK" +
		" LEFT JOIN ZSFNOTE nt ON b.ZLINKINGTO = nt.Z_PK"
	var args []any

	if lastSyncAt > 0 {
		query += " WHERE b.ZMODIFICATIONDATE >= ?"
		args = append(args, lastSyncAt)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query backlinks: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []mapper.BearBacklinkRow

	for rows.Next() {
		var row mapper.BearBacklinkRow
		if err := rows.Scan(
			&row.ZPK, &row.ZUNIQUEIDENTIFIER, &row.ZLINKEDBY, &row.ZLINKINGTO,
			&row.ZTITLE, &row.ZLOCATION, &row.ZVERSION, &row.ZMODIFICATIONDATE,
		); err != nil {
			return nil, fmt.Errorf("scan backlink row: %w", err)
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backlink rows: %w", err)
	}

	return result, nil
}

// NoteTags returns all note-tag associations with resolved UUIDs.
func (s *SQLiteBearDB) NoteTags(ctx context.Context) ([]NoteTagPair, error) {
	return s.queryJunction(ctx,
		"SELECT n.ZUNIQUEIDENTIFIER, t.ZUNIQUEIDENTIFIER"+
			" FROM Z_5TAGS j"+
			" JOIN ZSFNOTE n ON j.Z_5NOTES = n.Z_PK"+
			" JOIN ZSFNOTETAG t ON j.Z_13TAGS = t.Z_PK"+
			" WHERE n.ZUNIQUEIDENTIFIER IS NOT NULL AND t.ZUNIQUEIDENTIFIER IS NOT NULL",
	)
}

// PinnedNoteTags returns all pinned note-tag associations with resolved UUIDs.
func (s *SQLiteBearDB) PinnedNoteTags(ctx context.Context) ([]NoteTagPair, error) {
	return s.queryJunction(ctx,
		"SELECT n.ZUNIQUEIDENTIFIER, t.ZUNIQUEIDENTIFIER"+
			" FROM Z_5PINNEDINTAGS j"+
			" JOIN ZSFNOTE n ON j.Z_5PINNEDNOTES = n.Z_PK"+
			" JOIN ZSFNOTETAG t ON j.Z_13PINNEDINTAGS = t.Z_PK"+
			" WHERE n.ZUNIQUEIDENTIFIER IS NOT NULL AND t.ZUNIQUEIDENTIFIER IS NOT NULL",
	)
}

// NoteTagsForNotes returns note-tag associations for specific note UUIDs.
func (s *SQLiteBearDB) NoteTagsForNotes(ctx context.Context, noteUUIDs []string) ([]NoteTagPair, error) {
	if len(noteUUIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(noteUUIDs))
	args := make([]any, len(noteUUIDs))

	for i, uuid := range noteUUIDs {
		placeholders[i] = "?"
		args[i] = uuid
	}

	//nolint:gosec // placeholders are generated from len(noteUUIDs), not user input
	query := "SELECT n.ZUNIQUEIDENTIFIER, t.ZUNIQUEIDENTIFIER" +
		" FROM Z_5TAGS j" +
		" JOIN ZSFNOTE n ON j.Z_5NOTES = n.Z_PK" +
		" JOIN ZSFNOTETAG t ON j.Z_13TAGS = t.Z_PK" +
		" WHERE n.ZUNIQUEIDENTIFIER IN (" + strings.Join(placeholders, ",") + ")" +
		" AND t.ZUNIQUEIDENTIFIER IS NOT NULL"

	return s.queryJunction(ctx, query, args...)
}

// PinnedNoteTagsForNotes returns pinned note-tag associations for specific note UUIDs.
func (s *SQLiteBearDB) PinnedNoteTagsForNotes(ctx context.Context, noteUUIDs []string) ([]NoteTagPair, error) {
	if len(noteUUIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(noteUUIDs))
	args := make([]any, len(noteUUIDs))

	for i, uuid := range noteUUIDs {
		placeholders[i] = "?"
		args[i] = uuid
	}

	//nolint:gosec // placeholders are generated from len(noteUUIDs), not user input
	query := "SELECT n.ZUNIQUEIDENTIFIER, t.ZUNIQUEIDENTIFIER" +
		" FROM Z_5PINNEDINTAGS j" +
		" JOIN ZSFNOTE n ON j.Z_5PINNEDNOTES = n.Z_PK" +
		" JOIN ZSFNOTETAG t ON j.Z_13PINNEDINTAGS = t.Z_PK" +
		" WHERE n.ZUNIQUEIDENTIFIER IN (" + strings.Join(placeholders, ",") + ")" +
		" AND t.ZUNIQUEIDENTIFIER IS NOT NULL"

	return s.queryJunction(ctx, query, args...)
}

// AllNoteUUIDs returns UUIDs of all notes.
func (s *SQLiteBearDB) AllNoteUUIDs(ctx context.Context) ([]string, error) {
	return s.queryUUIDs(ctx, "SELECT ZUNIQUEIDENTIFIER FROM ZSFNOTE WHERE ZUNIQUEIDENTIFIER IS NOT NULL")
}

// AllTagUUIDs returns UUIDs of all tags.
func (s *SQLiteBearDB) AllTagUUIDs(ctx context.Context) ([]string, error) {
	return s.queryUUIDs(ctx, "SELECT ZUNIQUEIDENTIFIER FROM ZSFNOTETAG WHERE ZUNIQUEIDENTIFIER IS NOT NULL")
}

// AllAttachmentUUIDs returns UUIDs of all attachments.
func (s *SQLiteBearDB) AllAttachmentUUIDs(ctx context.Context) ([]string, error) {
	return s.queryUUIDs(ctx, "SELECT ZUNIQUEIDENTIFIER FROM ZSFNOTEFILE WHERE ZUNIQUEIDENTIFIER IS NOT NULL")
}

// AllBacklinkUUIDs returns UUIDs of all backlinks.
func (s *SQLiteBearDB) AllBacklinkUUIDs(ctx context.Context) ([]string, error) {
	return s.queryUUIDs(ctx,
		"SELECT ZUNIQUEIDENTIFIER FROM ZSFNOTEBACKLINK WHERE ZUNIQUEIDENTIFIER IS NOT NULL",
	)
}

// NoteByUUID returns basic note info by Bear UUID for write queue verification.
func (s *SQLiteBearDB) NoteByUUID(ctx context.Context, bearUUID string) (*NoteBasicInfo, error) {
	var info NoteBasicInfo
	var title, body sql.NullString
	var trashed, archived sql.NullInt64
	var modifiedAt sql.NullFloat64

	err := s.db.QueryRowContext(ctx,
		"SELECT ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT, ZMODIFICATIONDATE, ZTRASHED, ZARCHIVED FROM ZSFNOTE WHERE ZUNIQUEIDENTIFIER = ?",
		bearUUID,
	).Scan(&info.UUID, &title, &body, &modifiedAt, &trashed, &archived)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query note by uuid: %w", err)
	}

	if title.Valid {
		info.Title = title.String
	}
	if body.Valid {
		info.Body = body.String
	}
	if modifiedAt.Valid {
		info.ModifiedAt = modifiedAt.Float64
	}
	if trashed.Valid {
		info.Trashed = trashed.Int64
	}
	if archived.Valid {
		info.Archived = archived.Int64
	}

	return &info, nil
}

// NoteTagTitles returns tag titles for a note identified by Bear UUID.
func (s *SQLiteBearDB) NoteTagTitles(ctx context.Context, bearUUID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT t.ZTITLE FROM Z_5TAGS j"+
			" JOIN ZSFNOTE n ON j.Z_5NOTES = n.Z_PK"+
			" JOIN ZSFNOTETAG t ON j.Z_13TAGS = t.Z_PK"+
			" WHERE n.ZUNIQUEIDENTIFIER = ? AND t.ZTITLE IS NOT NULL",
		bearUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("query note tag titles: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, fmt.Errorf("scan tag title: %w", err)
		}
		result = append(result, title)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag titles: %w", err)
	}

	return result, nil
}

// NoteAttachmentFilenames returns filenames of attachments for a note identified by Bear UUID.
func (s *SQLiteBearDB) NoteAttachmentFilenames(ctx context.Context, bearUUID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT f.ZFILENAME FROM ZSFNOTEFILE f"+
			" JOIN ZSFNOTE n ON f.ZNOTE = n.Z_PK"+
			" WHERE n.ZUNIQUEIDENTIFIER = ? AND f.ZFILENAME IS NOT NULL AND COALESCE(f.ZPERMANENTLYDELETED, 0) = 0",
		bearUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("query note attachment filenames: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []string
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("scan attachment filename: %w", err)
		}
		result = append(result, filename)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment filenames: %w", err)
	}

	return result, nil
}

// FindRecentNotesByTitle finds notes by title created after the given Core Data epoch timestamp.
func (s *SQLiteBearDB) FindRecentNotesByTitle(
	ctx context.Context, title string, createdAfter float64,
) ([]NoteBasicInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT, ZMODIFICATIONDATE, ZTRASHED FROM ZSFNOTE"+
			" WHERE ZTITLE = ? AND ZCREATIONDATE > ?",
		title, createdAfter,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent notes by title: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []NoteBasicInfo
	for rows.Next() {
		var info NoteBasicInfo
		var noteTitle, body sql.NullString
		var modifiedAt sql.NullFloat64
		var trashed sql.NullInt64

		if err := rows.Scan(&info.UUID, &noteTitle, &body, &modifiedAt, &trashed); err != nil {
			return nil, fmt.Errorf("scan recent note: %w", err)
		}
		if noteTitle.Valid {
			info.Title = noteTitle.String
		}
		if body.Valid {
			info.Body = body.String
		}
		if modifiedAt.Valid {
			info.ModifiedAt = modifiedAt.Float64
		}
		if trashed.Valid {
			info.Trashed = trashed.Int64
		}
		result = append(result, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent notes: %w", err)
	}

	return result, nil
}

// queryJunction executes a junction table query with optional args and returns NoteTagPair results.
func (s *SQLiteBearDB) queryJunction(ctx context.Context, query string, args ...any) ([]NoteTagPair, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query junction: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []NoteTagPair

	for rows.Next() {
		var pair NoteTagPair
		if err := rows.Scan(&pair.NoteUUID, &pair.TagUUID); err != nil {
			return nil, fmt.Errorf("scan junction row: %w", err)
		}

		result = append(result, pair)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate junction rows: %w", err)
	}

	return result, nil
}

// queryUUIDs executes a query returning a single string column and collects results.
func (s *SQLiteBearDB) queryUUIDs(ctx context.Context, query string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query uuids: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only query

	var result []string

	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, fmt.Errorf("scan uuid: %w", err)
		}

		result = append(result, uuid)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate uuid rows: %w", err)
	}

	return result, nil
}
