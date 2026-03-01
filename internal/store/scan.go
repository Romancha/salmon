package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/romancha/bear-sync/internal/models"
)

// --- Note columns and scanning ---

var noteColumnList = []string{
	"rowid", "id", "bear_id", "title", "subtitle", "body",
	"archived", "encrypted", "has_files", "has_images", "has_source_code",
	"locked", "pinned", "shown_in_today", "trashed", "permanently_deleted",
	"skip_sync", "todo_completed", "todo_incompleted", "version",
	"created_at", "modified_at", "archived_at", "encrypted_at", "locked_at",
	"pinned_at", "trashed_at", "order_date", "conflict_id_date",
	"last_editing_device", "conflict_id", "encryption_id", "encrypted_data",
	"sync_status", "hub_modified_at", "bear_raw",
}

func noteColumns() string {
	return strings.Join(noteColumnList, ", ")
}

func noteColumnsWithPrefix(prefix string) string {
	cols := make([]string, len(noteColumnList))
	for i, c := range noteColumnList {
		cols[i] = prefix + c
	}

	return strings.Join(cols, ", ")
}

type noteScanner struct {
	RowID              int64
	ID                 string
	BearID             sql.NullString
	Title              sql.NullString
	Subtitle           sql.NullString
	Body               sql.NullString
	Archived           sql.NullInt64
	Encrypted          sql.NullInt64
	HasFiles           sql.NullInt64
	HasImages          sql.NullInt64
	HasSourceCode      sql.NullInt64
	Locked             sql.NullInt64
	Pinned             sql.NullInt64
	ShownInToday       sql.NullInt64
	Trashed            sql.NullInt64
	PermanentlyDeleted sql.NullInt64
	SkipSync           sql.NullInt64
	TodoCompleted      sql.NullInt64
	TodoIncompleted    sql.NullInt64
	Version            sql.NullInt64
	CreatedAt          sql.NullString
	ModifiedAt         sql.NullString
	ArchivedAt         sql.NullString
	EncryptedAt        sql.NullString
	LockedAt           sql.NullString
	PinnedAt           sql.NullString
	TrashedAt          sql.NullString
	OrderDate          sql.NullString
	ConflictIDDate     sql.NullString
	LastEditingDevice  sql.NullString
	ConflictID         sql.NullString
	EncryptionID       sql.NullString
	EncryptedData      []byte
	SyncStatus         sql.NullString
	HubModifiedAt      sql.NullString
	BearRaw            sql.NullString
}

func (ns *noteScanner) dest() []any {
	return []any{
		&ns.RowID, &ns.ID, &ns.BearID, &ns.Title, &ns.Subtitle, &ns.Body,
		&ns.Archived, &ns.Encrypted, &ns.HasFiles, &ns.HasImages,
		&ns.HasSourceCode, &ns.Locked, &ns.Pinned, &ns.ShownInToday,
		&ns.Trashed, &ns.PermanentlyDeleted, &ns.SkipSync,
		&ns.TodoCompleted, &ns.TodoIncompleted, &ns.Version,
		&ns.CreatedAt, &ns.ModifiedAt, &ns.ArchivedAt, &ns.EncryptedAt,
		&ns.LockedAt, &ns.PinnedAt, &ns.TrashedAt, &ns.OrderDate,
		&ns.ConflictIDDate, &ns.LastEditingDevice, &ns.ConflictID,
		&ns.EncryptionID, &ns.EncryptedData, &ns.SyncStatus,
		&ns.HubModifiedAt, &ns.BearRaw,
	}
}

func (ns *noteScanner) toNote() models.Note {
	n := models.Note{
		RowID:              ns.RowID,
		ID:                 ns.ID,
		Title:              ns.Title.String,
		Subtitle:           ns.Subtitle.String,
		Body:               ns.Body.String,
		Archived:           int(ns.Archived.Int64),
		Encrypted:          int(ns.Encrypted.Int64),
		HasFiles:           int(ns.HasFiles.Int64),
		HasImages:          int(ns.HasImages.Int64),
		HasSourceCode:      int(ns.HasSourceCode.Int64),
		Locked:             int(ns.Locked.Int64),
		Pinned:             int(ns.Pinned.Int64),
		ShownInToday:       int(ns.ShownInToday.Int64),
		Trashed:            int(ns.Trashed.Int64),
		PermanentlyDeleted: int(ns.PermanentlyDeleted.Int64),
		SkipSync:           int(ns.SkipSync.Int64),
		TodoCompleted:      int(ns.TodoCompleted.Int64),
		TodoIncompleted:    int(ns.TodoIncompleted.Int64),
		Version:            int(ns.Version.Int64),
		CreatedAt:          ns.CreatedAt.String,
		ModifiedAt:         ns.ModifiedAt.String,
		ArchivedAt:         ns.ArchivedAt.String,
		EncryptedAt:        ns.EncryptedAt.String,
		LockedAt:           ns.LockedAt.String,
		PinnedAt:           ns.PinnedAt.String,
		TrashedAt:          ns.TrashedAt.String,
		OrderDate:          ns.OrderDate.String,
		ConflictIDDate:     ns.ConflictIDDate.String,
		LastEditingDevice:  ns.LastEditingDevice.String,
		ConflictID:         ns.ConflictID.String,
		EncryptionID:       ns.EncryptionID.String,
		EncryptedData:      ns.EncryptedData,
		SyncStatus:         ns.SyncStatus.String,
		HubModifiedAt:      ns.HubModifiedAt.String,
		BearRaw:            ns.BearRaw.String,
	}
	if ns.BearID.Valid {
		s := ns.BearID.String
		n.BearID = &s
	}

	return n
}

func scanNote(rows *sql.Rows) (models.Note, error) {
	var ns noteScanner
	if err := rows.Scan(ns.dest()...); err != nil {
		return models.Note{}, fmt.Errorf("scan note: %w", err)
	}

	return ns.toNote(), nil
}

func scanNoteRow(row *sql.Row) (models.Note, error) {
	var ns noteScanner
	if err := row.Scan(ns.dest()...); err != nil {
		return models.Note{}, fmt.Errorf("scan note row: %w", err)
	}

	return ns.toNote(), nil
}

func noteValues(n *models.Note) []any {
	return []any{
		n.ID, n.BearID, n.Title, n.Subtitle, n.Body,
		n.Archived, n.Encrypted, n.HasFiles, n.HasImages, n.HasSourceCode,
		n.Locked, n.Pinned, n.ShownInToday, n.Trashed, n.PermanentlyDeleted,
		n.SkipSync, n.TodoCompleted, n.TodoIncompleted, n.Version,
		n.CreatedAt, n.ModifiedAt, n.ArchivedAt, n.EncryptedAt, n.LockedAt,
		n.PinnedAt, n.TrashedAt, n.OrderDate, n.ConflictIDDate,
		n.LastEditingDevice, n.ConflictID, n.EncryptionID, n.EncryptedData,
		n.SyncStatus, n.HubModifiedAt, n.BearRaw,
	}
}

// --- Tag columns and scanning ---

var tagColumnList = []string{
	"id", "bear_id", "title", "pinned", "is_root", "hide_subtags_notes",
	"sorting", "sorting_direction", "encrypted", "version",
	"modified_at", "pinned_at", "pinned_notes_at", "encrypted_at",
	"hide_subtags_notes_at", "sorting_at", "sorting_direction_at",
	"tag_con_date", "tag_con", "bear_raw",
}

func tagColumns() string {
	return strings.Join(tagColumnList, ", ")
}

func tagColumnsWithPrefix(prefix string) string {
	cols := make([]string, len(tagColumnList))
	for i, c := range tagColumnList {
		cols[i] = prefix + c
	}

	return strings.Join(cols, ", ")
}

type tagScanner struct {
	ID                 string
	BearID             sql.NullString
	Title              sql.NullString
	Pinned             sql.NullInt64
	IsRoot             sql.NullInt64
	HideSubtagsNotes   sql.NullInt64
	Sorting            sql.NullInt64
	SortingDirection   sql.NullInt64
	Encrypted          sql.NullInt64
	Version            sql.NullInt64
	ModifiedAt         sql.NullString
	PinnedAt           sql.NullString
	PinnedNotesAt      sql.NullString
	EncryptedAt        sql.NullString
	HideSubtagsNotesAt sql.NullString
	SortingAt          sql.NullString
	SortingDirectionAt sql.NullString
	TagConDate         sql.NullString
	TagCon             sql.NullString
	BearRaw            sql.NullString
}

func (ts *tagScanner) dest() []any {
	return []any{
		&ts.ID, &ts.BearID, &ts.Title, &ts.Pinned, &ts.IsRoot,
		&ts.HideSubtagsNotes, &ts.Sorting, &ts.SortingDirection,
		&ts.Encrypted, &ts.Version, &ts.ModifiedAt, &ts.PinnedAt,
		&ts.PinnedNotesAt, &ts.EncryptedAt, &ts.HideSubtagsNotesAt,
		&ts.SortingAt, &ts.SortingDirectionAt, &ts.TagConDate,
		&ts.TagCon, &ts.BearRaw,
	}
}

func (ts *tagScanner) toTag() models.Tag {
	t := models.Tag{
		ID:                 ts.ID,
		Title:              ts.Title.String,
		Pinned:             int(ts.Pinned.Int64),
		IsRoot:             int(ts.IsRoot.Int64),
		HideSubtagsNotes:   int(ts.HideSubtagsNotes.Int64),
		Sorting:            int(ts.Sorting.Int64),
		SortingDirection:   int(ts.SortingDirection.Int64),
		Encrypted:          int(ts.Encrypted.Int64),
		Version:            int(ts.Version.Int64),
		ModifiedAt:         ts.ModifiedAt.String,
		PinnedAt:           ts.PinnedAt.String,
		PinnedNotesAt:      ts.PinnedNotesAt.String,
		EncryptedAt:        ts.EncryptedAt.String,
		HideSubtagsNotesAt: ts.HideSubtagsNotesAt.String,
		SortingAt:          ts.SortingAt.String,
		SortingDirectionAt: ts.SortingDirectionAt.String,
		TagConDate:         ts.TagConDate.String,
		TagCon:             ts.TagCon.String,
		BearRaw:            ts.BearRaw.String,
	}
	if ts.BearID.Valid {
		s := ts.BearID.String
		t.BearID = &s
	}

	return t
}

func scanTag(rows *sql.Rows) (models.Tag, error) {
	var ts tagScanner
	if err := rows.Scan(ts.dest()...); err != nil {
		return models.Tag{}, fmt.Errorf("scan tag: %w", err)
	}

	return ts.toTag(), nil
}

func scanTagRow(row *sql.Row) (models.Tag, error) {
	var ts tagScanner
	if err := row.Scan(ts.dest()...); err != nil {
		return models.Tag{}, fmt.Errorf("scan tag row: %w", err)
	}

	return ts.toTag(), nil
}

func tagValues(t *models.Tag) []any {
	return []any{
		t.ID, t.BearID, t.Title, t.Pinned, t.IsRoot, t.HideSubtagsNotes,
		t.Sorting, t.SortingDirection, t.Encrypted, t.Version,
		t.ModifiedAt, t.PinnedAt, t.PinnedNotesAt, t.EncryptedAt,
		t.HideSubtagsNotesAt, t.SortingAt, t.SortingDirectionAt,
		t.TagConDate, t.TagCon, t.BearRaw,
	}
}

// --- Attachment columns and scanning ---

var attachmentColumnList = []string{
	"id", "bear_id", "note_id", "type", "filename", "normalized_extension",
	"file_size", "file_index",
	"width", "height", "animated", "duration", "width1", "height1",
	"downloaded", "encrypted", "permanently_deleted", "skip_sync",
	"unused", "uploaded", "version",
	"created_at", "modified_at", "inserted_at", "encrypted_at",
	"unused_at", "uploaded_at", "search_text_at",
	"last_editing_device", "encryption_id", "search_text", "encrypted_data",
	"file_path", "bear_raw",
}

func attachmentColumns() string {
	return strings.Join(attachmentColumnList, ", ")
}

type attachmentScanner struct {
	ID                  string
	BearID              sql.NullString
	NoteID              sql.NullString
	Type                sql.NullString
	Filename            sql.NullString
	NormalizedExtension sql.NullString
	FileSize            sql.NullInt64
	FileIndex           sql.NullInt64
	Width               sql.NullInt64
	Height              sql.NullInt64
	Animated            sql.NullInt64
	Duration            sql.NullInt64
	Width1              sql.NullInt64
	Height1             sql.NullInt64
	Downloaded          sql.NullInt64
	Encrypted           sql.NullInt64
	PermanentlyDeleted  sql.NullInt64
	SkipSync            sql.NullInt64
	Unused              sql.NullInt64
	Uploaded            sql.NullInt64
	Version             sql.NullInt64
	CreatedAt           sql.NullString
	ModifiedAt          sql.NullString
	InsertedAt          sql.NullString
	EncryptedAt         sql.NullString
	UnusedAt            sql.NullString
	UploadedAt          sql.NullString
	SearchTextAt        sql.NullString
	LastEditingDevice   sql.NullString
	EncryptionID        sql.NullString
	SearchText          sql.NullString
	EncryptedData       []byte
	FilePath            sql.NullString
	BearRaw             sql.NullString
}

func (as *attachmentScanner) dest() []any {
	return []any{
		&as.ID, &as.BearID, &as.NoteID, &as.Type, &as.Filename,
		&as.NormalizedExtension, &as.FileSize, &as.FileIndex,
		&as.Width, &as.Height, &as.Animated, &as.Duration,
		&as.Width1, &as.Height1, &as.Downloaded, &as.Encrypted,
		&as.PermanentlyDeleted, &as.SkipSync, &as.Unused, &as.Uploaded,
		&as.Version, &as.CreatedAt, &as.ModifiedAt, &as.InsertedAt,
		&as.EncryptedAt, &as.UnusedAt, &as.UploadedAt, &as.SearchTextAt,
		&as.LastEditingDevice, &as.EncryptionID, &as.SearchText,
		&as.EncryptedData, &as.FilePath, &as.BearRaw,
	}
}

func (as *attachmentScanner) toAttachment() models.Attachment {
	a := models.Attachment{
		ID:                  as.ID,
		NoteID:              as.NoteID.String,
		Type:                as.Type.String,
		Filename:            as.Filename.String,
		NormalizedExtension: as.NormalizedExtension.String,
		FileSize:            as.FileSize.Int64,
		FileIndex:           int(as.FileIndex.Int64),
		Width:               int(as.Width.Int64),
		Height:              int(as.Height.Int64),
		Animated:            int(as.Animated.Int64),
		Duration:            int(as.Duration.Int64),
		Width1:              int(as.Width1.Int64),
		Height1:             int(as.Height1.Int64),
		Downloaded:          int(as.Downloaded.Int64),
		Encrypted:           int(as.Encrypted.Int64),
		PermanentlyDeleted:  int(as.PermanentlyDeleted.Int64),
		SkipSync:            int(as.SkipSync.Int64),
		Unused:              int(as.Unused.Int64),
		Uploaded:            int(as.Uploaded.Int64),
		Version:             int(as.Version.Int64),
		CreatedAt:           as.CreatedAt.String,
		ModifiedAt:          as.ModifiedAt.String,
		InsertedAt:          as.InsertedAt.String,
		EncryptedAt:         as.EncryptedAt.String,
		UnusedAt:            as.UnusedAt.String,
		UploadedAt:          as.UploadedAt.String,
		SearchTextAt:        as.SearchTextAt.String,
		LastEditingDevice:   as.LastEditingDevice.String,
		EncryptionID:        as.EncryptionID.String,
		SearchText:          as.SearchText.String,
		EncryptedData:       as.EncryptedData,
		FilePath:            as.FilePath.String,
		BearRaw:             as.BearRaw.String,
	}
	if as.BearID.Valid {
		s := as.BearID.String
		a.BearID = &s
	}

	return a
}

func scanAttachment(rows *sql.Rows) (models.Attachment, error) {
	var as attachmentScanner
	if err := rows.Scan(as.dest()...); err != nil {
		return models.Attachment{}, fmt.Errorf("scan attachment: %w", err)
	}

	return as.toAttachment(), nil
}

func scanAttachmentRow(row *sql.Row) (models.Attachment, error) {
	var as attachmentScanner
	if err := row.Scan(as.dest()...); err != nil {
		return models.Attachment{}, fmt.Errorf("scan attachment row: %w", err)
	}

	return as.toAttachment(), nil
}

func attachmentValues(a *models.Attachment) []any {
	return []any{
		a.ID, a.BearID, a.NoteID, a.Type, a.Filename, a.NormalizedExtension,
		a.FileSize, a.FileIndex,
		a.Width, a.Height, a.Animated, a.Duration, a.Width1, a.Height1,
		a.Downloaded, a.Encrypted, a.PermanentlyDeleted, a.SkipSync,
		a.Unused, a.Uploaded, a.Version,
		a.CreatedAt, a.ModifiedAt, a.InsertedAt, a.EncryptedAt,
		a.UnusedAt, a.UploadedAt, a.SearchTextAt,
		a.LastEditingDevice, a.EncryptionID, a.SearchText, a.EncryptedData,
		a.FilePath, a.BearRaw,
	}
}

// --- Backlink columns and scanning ---

var backlinkColumnList = []string{
	"id", "bear_id", "linked_by_id", "linking_to_id",
	"title", "location", "version", "modified_at", "bear_raw",
}

func backlinkColumns() string {
	return strings.Join(backlinkColumnList, ", ")
}

type backlinkScanner struct {
	ID          string
	BearID      sql.NullString
	LinkedByID  sql.NullString
	LinkingToID sql.NullString
	Title       sql.NullString
	Location    sql.NullInt64
	Version     sql.NullInt64
	ModifiedAt  sql.NullString
	BearRaw     sql.NullString
}

func (bs *backlinkScanner) dest() []any {
	return []any{
		&bs.ID, &bs.BearID, &bs.LinkedByID, &bs.LinkingToID,
		&bs.Title, &bs.Location, &bs.Version, &bs.ModifiedAt, &bs.BearRaw,
	}
}

func (bs *backlinkScanner) toBacklink() models.Backlink {
	b := models.Backlink{
		ID:          bs.ID,
		LinkedByID:  bs.LinkedByID.String,
		LinkingToID: bs.LinkingToID.String,
		Title:       bs.Title.String,
		Location:    int(bs.Location.Int64),
		Version:     int(bs.Version.Int64),
		ModifiedAt:  bs.ModifiedAt.String,
		BearRaw:     bs.BearRaw.String,
	}
	if bs.BearID.Valid {
		s := bs.BearID.String
		b.BearID = &s
	}

	return b
}

func scanBacklink(rows *sql.Rows) (models.Backlink, error) {
	var bs backlinkScanner
	if err := rows.Scan(bs.dest()...); err != nil {
		return models.Backlink{}, fmt.Errorf("scan backlink: %w", err)
	}

	return bs.toBacklink(), nil
}

func backlinkValues(b *models.Backlink) []any {
	return []any{
		b.ID, b.BearID, b.LinkedByID, b.LinkingToID,
		b.Title, b.Location, b.Version, b.ModifiedAt, b.BearRaw,
	}
}

// --- Write Queue columns and scanning ---

var writeQueueColumnList = []string{
	"id", "idempotency_key", "action", "note_id", "payload",
	"created_at", "status", "processing_by", "lease_until",
	"applied_at", "error",
}

func writeQueueColumns() string {
	return strings.Join(writeQueueColumnList, ", ")
}

func prefixedWriteQueueColumns(prefix string) string {
	cols := make([]string, len(writeQueueColumnList))
	for i, c := range writeQueueColumnList {
		cols[i] = prefix + "." + c
	}

	return strings.Join(cols, ", ")
}

type writeQueueScanner struct {
	ID             int64
	IdempotencyKey string
	Action         sql.NullString
	NoteID         sql.NullString
	Payload        sql.NullString
	CreatedAt      sql.NullString
	Status         sql.NullString
	ProcessingBy   sql.NullString
	LeaseUntil     sql.NullString
	AppliedAt      sql.NullString
	Error          sql.NullString
}

func (ws *writeQueueScanner) dest() []any {
	return []any{
		&ws.ID, &ws.IdempotencyKey, &ws.Action, &ws.NoteID, &ws.Payload,
		&ws.CreatedAt, &ws.Status, &ws.ProcessingBy, &ws.LeaseUntil,
		&ws.AppliedAt, &ws.Error,
	}
}

func (ws *writeQueueScanner) toItem() models.WriteQueueItem {
	return models.WriteQueueItem{
		ID:             ws.ID,
		IdempotencyKey: ws.IdempotencyKey,
		Action:         ws.Action.String,
		NoteID:         ws.NoteID.String,
		Payload:        ws.Payload.String,
		CreatedAt:      ws.CreatedAt.String,
		Status:         ws.Status.String,
		ProcessingBy:   ws.ProcessingBy.String,
		LeaseUntil:     ws.LeaseUntil.String,
		AppliedAt:      ws.AppliedAt.String,
		Error:          ws.Error.String,
	}
}

func scanWriteQueueRow(row *sql.Row) (models.WriteQueueItem, error) {
	var ws writeQueueScanner
	if err := row.Scan(ws.dest()...); err != nil {
		return models.WriteQueueItem{}, fmt.Errorf("scan write queue row: %w", err)
	}

	return ws.toItem(), nil
}

func scanWriteQueueWithSyncStatus(rows *sql.Rows) (models.WriteQueueItem, error) {
	var ws writeQueueScanner
	var noteSyncStatus sql.NullString

	dest := append(ws.dest(), &noteSyncStatus) //nolint:gocritic // append to new slice is intentional
	if err := rows.Scan(dest...); err != nil {
		return models.WriteQueueItem{}, fmt.Errorf("scan write queue with sync status: %w", err)
	}

	item := ws.toItem()
	item.NoteSyncStatus = noteSyncStatus.String

	return item, nil
}
