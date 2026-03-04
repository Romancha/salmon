package beardb_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/beardb"

	_ "modernc.org/sqlite"
)

// coreDataEpochOffset for test date calculations.
const coreDataEpochOffset = 978307200

// coreDataDate converts a Unix timestamp to Core Data epoch.
func coreDataDate(unixSec int64) float64 {
	return float64(unixSec - coreDataEpochOffset)
}

// setupTestBearDB creates a temporary Bear SQLite database with test data.
func setupTestBearDB(t *testing.T) *beardb.SQLiteBearDB {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bear.sqlite")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	createBearSchema(t, db)
	insertTestData(t, db)

	require.NoError(t, db.Close())

	bearDB, err := beardb.New(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, bearDB.Close())
	})

	return bearDB
}

func createBearSchema(t *testing.T, db *sql.DB) { //nolint:thelper // used as test helper and fixture generator
	ctx := context.Background()

	statements := []string{
		`CREATE TABLE ZSFNOTE (
			Z_PK INTEGER PRIMARY KEY,
			ZUNIQUEIDENTIFIER TEXT,
			ZTITLE TEXT,
			ZSUBTITLE TEXT,
			ZTEXT TEXT,
			ZARCHIVED INTEGER,
			ZENCRYPTED INTEGER,
			ZHASFILES INTEGER,
			ZHASIMAGES INTEGER,
			ZHASSOURCECODE INTEGER,
			ZLOCKED INTEGER,
			ZPINNED INTEGER,
			ZSHOWNINTODAYWIDGET INTEGER,
			ZTRASHED INTEGER,
			ZPERMANENTLYDELETED INTEGER,
			ZSKIPSYNC INTEGER,
			ZTODOCOMPLETED INTEGER,
			ZTODOINCOMPLETED INTEGER,
			ZVERSION INTEGER,
			ZCREATIONDATE REAL,
			ZMODIFICATIONDATE REAL,
			ZARCHIVEDDATE REAL,
			ZENCRYPTIONDATE REAL,
			ZLOCKEDDATE REAL,
			ZPINNEDDATE REAL,
			ZTRASHEDDATE REAL,
			ZORDERDATE REAL,
			ZCONFLICTUNIQUEIDENTIFIERDATE REAL,
			ZLASTEDITINGDEVICE TEXT,
			ZCONFLICTUNIQUEIDENTIFIER TEXT,
			ZENCRYPTIONUNIQUEIDENTIFIER TEXT,
			ZENCRYPTEDDATA BLOB
		)`,
		`CREATE TABLE ZSFNOTETAG (
			Z_PK INTEGER PRIMARY KEY,
			ZUNIQUEIDENTIFIER TEXT,
			ZTITLE TEXT,
			ZPINNED INTEGER,
			ZISROOT INTEGER,
			ZHIDESUBTAGSNOTES INTEGER,
			ZSORTING INTEGER,
			ZSORTINGDIRECTION INTEGER,
			ZENCRYPTED INTEGER,
			ZVERSION INTEGER,
			ZMODIFICATIONDATE REAL,
			ZPINNEDDATE REAL,
			ZPINNEDNOTESDATE REAL,
			ZENCRYPTEDDATE REAL,
			ZHIDESUBTAGSNOTESDATE REAL,
			ZSORTINGDATE REAL,
			ZSORTINGDIRECTIONDATE REAL,
			ZTAGCONDATE REAL,
			ZTAGCON TEXT
		)`,
		`CREATE TABLE ZSFNOTEFILE (
			Z_PK INTEGER PRIMARY KEY,
			Z_ENT INTEGER,
			ZUNIQUEIDENTIFIER TEXT,
			ZNOTE INTEGER,
			ZFILENAME TEXT,
			ZNORMALIZEDFILEEXTENSION TEXT,
			ZFILESIZE INTEGER,
			ZINDEX INTEGER,
			ZWIDTH INTEGER,
			ZHEIGHT INTEGER,
			ZANIMATED INTEGER,
			ZDURATION INTEGER,
			ZWIDTH1 INTEGER,
			ZHEIGHT1 INTEGER,
			ZDOWNLOADED INTEGER,
			ZENCRYPTED INTEGER,
			ZPERMANENTLYDELETED INTEGER,
			ZSKIPSYNC INTEGER,
			ZUNUSED INTEGER,
			ZUPLOADED INTEGER,
			ZVERSION INTEGER,
			ZCREATIONDATE REAL,
			ZMODIFICATIONDATE REAL,
			ZINSERTIONDATE REAL,
			ZENCRYPTIONDATE REAL,
			ZUNUSEDDATE REAL,
			ZUPLOADEDDATE REAL,
			ZSEARCHTEXTDATE REAL,
			ZLASTEDITINGDEVICE TEXT,
			ZENCRYPTIONUNIQUEIDENTIFIER TEXT,
			ZSEARCHTEXT TEXT,
			ZENCRYPTEDDATA BLOB
		)`,
		`CREATE TABLE ZSFNOTEBACKLINK (
			Z_PK INTEGER PRIMARY KEY,
			ZUNIQUEIDENTIFIER TEXT,
			ZLINKEDBY INTEGER,
			ZLINKINGTO INTEGER,
			ZTITLE TEXT,
			ZLOCATION INTEGER,
			ZVERSION INTEGER,
			ZMODIFICATIONDATE REAL
		)`,
		`CREATE TABLE Z_5TAGS (
			Z_5NOTES INTEGER,
			Z_13TAGS INTEGER
		)`,
		`CREATE TABLE Z_5PINNEDINTAGS (
			Z_5PINNEDNOTES INTEGER,
			Z_13PINNEDINTAGS INTEGER
		)`,
	}

	for _, stmt := range statements {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err)
	}
}

// Test dates: 2024-01-15 12:00:00 UTC = Unix 1705320000 = Core Data 727012800.
// Earlier date: 2024-01-10 12:00:00 UTC = Unix 1704888000 = Core Data 726580800.
var (
	modDateRecent = coreDataDate(1705320000) // 2024-01-15 12:00:00 UTC
	modDateOld    = coreDataDate(1704888000) // 2024-01-10 12:00:00 UTC
	createDate    = coreDataDate(1704024000) // 2024-01-01 00:00:00 UTC
)

func insertTestData(t *testing.T, db *sql.DB) { //nolint:thelper // used as test helper and fixture generator
	ctx := context.Background()

	// Notes: 3 regular + 1 encrypted + 1 trashed
	notes := []string{
		`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT, ZENCRYPTED, ZTRASHED,
			ZPERMANENTLYDELETED, ZPINNED, ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE, ZLASTEDITINGDEVICE)
		VALUES (1, 'note-uuid-1', 'First Note', 'subtitle1', '# First Note\nHello world', 0, 0,
			0, 1, 3, ?, ?, 'MacBook')`,
		`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT, ZENCRYPTED, ZTRASHED,
			ZPERMANENTLYDELETED, ZPINNED, ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (2, 'note-uuid-2', 'Second Note', 'subtitle2', '# Second Note\nContent here', 0, 0,
			0, 0, 1, ?, ?)`,
		`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT, ZENCRYPTED, ZTRASHED,
			ZPERMANENTLYDELETED, ZPINNED, ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (3, 'note-uuid-3', 'Third Note', NULL, '# Third Note', 0, 0,
			0, 0, 1, ?, ?)`,
		// Encrypted note
		`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT, ZENCRYPTED, ZTRASHED,
			ZPERMANENTLYDELETED, ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE, ZENCRYPTEDDATA,
			ZENCRYPTIONUNIQUEIDENTIFIER)
		VALUES (4, 'note-uuid-enc', 'Encrypted Note', NULL, 1, 0,
			0, 1, ?, ?, X'DEADBEEF', 'enc-id-1')`,
		// Trashed note
		`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT, ZENCRYPTED, ZTRASHED,
			ZPERMANENTLYDELETED, ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE, ZTRASHEDDATE)
		VALUES (5, 'note-uuid-trash', 'Trashed Note', '# Trashed', 0, 1,
			0, 1, ?, ?, ?)`,
		// Note without UUID (edge case)
		`INSERT INTO ZSFNOTE (Z_PK, ZTITLE, ZTEXT, ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED,
			ZVERSION, ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (6, 'No UUID Note', 'body', 0, 0, 0, 1, ?, ?)`,
	}

	// Note 1: recent, Note 2: old, Note 3: recent, Note 4: old, Note 5: recent, Note 6: old
	noteArgs := [][]any{
		{createDate, modDateRecent},
		{createDate, modDateOld},
		{createDate, modDateRecent},
		{createDate, modDateOld},
		{createDate, modDateRecent, modDateRecent},
		{createDate, modDateOld},
	}

	for i, q := range notes {
		_, err := db.ExecContext(ctx, q, noteArgs[i]...)
		require.NoError(t, err)
	}

	// Tags: 2 regular + 1 hierarchical
	tags := []string{
		`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION,
			ZMODIFICATIONDATE)
		VALUES (1, 'tag-uuid-1', 'work', 0, 1, 1, ?)`,
		`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION,
			ZMODIFICATIONDATE)
		VALUES (2, 'tag-uuid-2', 'work/projects', 0, 0, 1, ?)`,
		`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION,
			ZMODIFICATIONDATE)
		VALUES (3, 'tag-uuid-3', 'personal', 1, 1, 2, ?)`,
	}

	tagDates := []float64{modDateRecent, modDateOld, modDateRecent}
	for i, q := range tags {
		_, err := db.ExecContext(ctx, q, tagDates[i])
		require.NoError(t, err)
	}

	// Attachments: 1 image + 1 file
	attachments := []string{
		`INSERT INTO ZSFNOTEFILE (Z_PK, Z_ENT, ZUNIQUEIDENTIFIER, ZNOTE, ZFILENAME,
			ZNORMALIZEDFILEEXTENSION, ZFILESIZE, ZWIDTH, ZHEIGHT, ZVERSION,
			ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (1, 9, 'att-uuid-1', 1, 'photo.jpg', 'jpg', 102400, 1920, 1080, 1, ?, ?)`,
		`INSERT INTO ZSFNOTEFILE (Z_PK, Z_ENT, ZUNIQUEIDENTIFIER, ZNOTE, ZFILENAME,
			ZNORMALIZEDFILEEXTENSION, ZFILESIZE, ZVERSION,
			ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (2, 8, 'att-uuid-2', 2, 'document.pdf', 'pdf', 51200, 1, ?, ?)`,
	}

	_, err := db.ExecContext(ctx, attachments[0], createDate, modDateRecent)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, attachments[1], createDate, modDateOld)
	require.NoError(t, err)

	// Backlinks: note 2 links to note 1
	_, err = db.ExecContext(ctx, `INSERT INTO ZSFNOTEBACKLINK (Z_PK, ZUNIQUEIDENTIFIER, ZLINKEDBY, ZLINKINGTO,
		ZTITLE, ZLOCATION, ZVERSION, ZMODIFICATIONDATE)
		VALUES (1, 'bl-uuid-1', 2, 1, 'First Note', 42, 1, ?)`, modDateRecent)
	require.NoError(t, err)

	// Junction tables: note1 -> tag1,tag2; note2 -> tag1; note3 -> tag3
	junctions := []string{
		`INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (1, 1)`,
		`INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (1, 2)`,
		`INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (2, 1)`,
		`INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (3, 3)`,
	}

	for _, q := range junctions {
		_, err := db.ExecContext(ctx, q)
		require.NoError(t, err)
	}

	// Pinned junction: note1 pinned in tag1
	_, err = db.ExecContext(ctx, `INSERT INTO Z_5PINNEDINTAGS (Z_5PINNEDNOTES, Z_13PINNEDINTAGS) VALUES (1, 1)`)
	require.NoError(t, err)
}

func TestNotes_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	notes, err := bearDB.Notes(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, notes, 6) // all 6 notes including no-uuid one
}

func TestNotes_Delta(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	// Only recent notes (modDateRecent)
	notes, err := bearDB.Notes(ctx, modDateRecent)
	require.NoError(t, err)
	assert.Len(t, notes, 3) // note 1, 3, 5 have recent dates

	// Check fields of first returned note
	var note1Found bool

	for _, n := range notes {
		if n.ZUNIQUEIDENTIFIER == nil || *n.ZUNIQUEIDENTIFIER != "note-uuid-1" {
			continue
		}

		note1Found = true
		assert.Equal(t, int64(1), n.ZPK)
		require.NotNil(t, n.ZTITLE)
		assert.Equal(t, "First Note", *n.ZTITLE)
		require.NotNil(t, n.ZTEXT)
		assert.Equal(t, "# First Note\\nHello world", *n.ZTEXT)
		require.NotNil(t, n.ZPINNED)
		assert.Equal(t, int64(1), *n.ZPINNED)
		require.NotNil(t, n.ZVERSION)
		assert.Equal(t, int64(3), *n.ZVERSION)
		require.NotNil(t, n.ZLASTEDITINGDEVICE)
		assert.Equal(t, "MacBook", *n.ZLASTEDITINGDEVICE)
	}

	assert.True(t, note1Found, "note-uuid-1 should be in delta results")
}

func TestNotes_EncryptedNote(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	notes, err := bearDB.Notes(ctx, 0)
	require.NoError(t, err)

	var encNote *struct{ found bool }

	for _, n := range notes {
		if n.ZUNIQUEIDENTIFIER == nil || *n.ZUNIQUEIDENTIFIER != "note-uuid-enc" {
			continue
		}

		encNote = &struct{ found bool }{found: true}
		require.NotNil(t, n.ZENCRYPTED)
		assert.Equal(t, int64(1), *n.ZENCRYPTED)
		assert.NotEmpty(t, n.ZENCRYPTEDDATA)
		require.NotNil(t, n.ZENCRYPTIONUNIQUEIDENTIFIER)
		assert.Equal(t, "enc-id-1", *n.ZENCRYPTIONUNIQUEIDENTIFIER)
		assert.Nil(t, n.ZTEXT) // text should be NULL for encrypted notes
	}

	require.NotNil(t, encNote, "encrypted note should exist")
}

func TestNotes_NullBearID(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	notes, err := bearDB.Notes(ctx, 0)
	require.NoError(t, err)

	var nullIDFound bool

	for _, n := range notes {
		if n.ZPK == 6 {
			nullIDFound = true
			assert.Nil(t, n.ZUNIQUEIDENTIFIER, "note 6 should have nil UUID")
		}
	}

	assert.True(t, nullIDFound, "note with NULL UUID should be returned")
}

func TestTags_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	tags, err := bearDB.Tags(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, tags, 3)
}

func TestTags_Delta(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	tags, err := bearDB.Tags(ctx, modDateRecent)
	require.NoError(t, err)
	assert.Len(t, tags, 2) // tag 1 and 3 have recent dates

	for _, tag := range tags {
		require.NotNil(t, tag.ZUNIQUEIDENTIFIER)
		assert.Contains(t, []string{"tag-uuid-1", "tag-uuid-3"}, *tag.ZUNIQUEIDENTIFIER)
	}
}

func TestTags_Fields(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	tags, err := bearDB.Tags(ctx, 0)
	require.NoError(t, err)

	for _, tag := range tags {
		if tag.ZUNIQUEIDENTIFIER != nil && *tag.ZUNIQUEIDENTIFIER == "tag-uuid-2" {
			require.NotNil(t, tag.ZTITLE)
			assert.Equal(t, "work/projects", *tag.ZTITLE, "hierarchical tag title")
			require.NotNil(t, tag.ZISROOT)
			assert.Equal(t, int64(0), *tag.ZISROOT, "child tag is not root")
		}

		if tag.ZUNIQUEIDENTIFIER != nil && *tag.ZUNIQUEIDENTIFIER == "tag-uuid-3" {
			require.NotNil(t, tag.ZPINNED)
			assert.Equal(t, int64(1), *tag.ZPINNED, "personal tag is pinned")
		}
	}
}

func TestAttachments_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	atts, err := bearDB.Attachments(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, atts, 2)
}

func TestAttachments_Delta(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	atts, err := bearDB.Attachments(ctx, modDateRecent)
	require.NoError(t, err)
	assert.Len(t, atts, 1)
	require.NotNil(t, atts[0].ZUNIQUEIDENTIFIER)
	assert.Equal(t, "att-uuid-1", *atts[0].ZUNIQUEIDENTIFIER)
}

func TestAttachments_NoteUUIDResolution(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	atts, err := bearDB.Attachments(ctx, 0)
	require.NoError(t, err)

	for _, att := range atts {
		if att.ZUNIQUEIDENTIFIER != nil && *att.ZUNIQUEIDENTIFIER == "att-uuid-1" {
			// ZNOTE should be resolved to note UUID via JOIN
			require.NotNil(t, att.ZNOTE)
			assert.Equal(t, "note-uuid-1", *att.ZNOTE, "attachment note should resolve to note UUID")
			assert.Equal(t, int64(9), att.ZENT, "should be image type")
			require.NotNil(t, att.ZWIDTH)
			assert.Equal(t, int64(1920), *att.ZWIDTH)
			require.NotNil(t, att.ZHEIGHT)
			assert.Equal(t, int64(1080), *att.ZHEIGHT)
		}

		if att.ZUNIQUEIDENTIFIER != nil && *att.ZUNIQUEIDENTIFIER == "att-uuid-2" {
			require.NotNil(t, att.ZNOTE)
			assert.Equal(t, "note-uuid-2", *att.ZNOTE)
			assert.Equal(t, int64(8), att.ZENT, "should be file type")
		}
	}
}

func TestBacklinks_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	bls, err := bearDB.Backlinks(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, bls, 1)

	bl := bls[0]
	require.NotNil(t, bl.ZUNIQUEIDENTIFIER)
	assert.Equal(t, "bl-uuid-1", *bl.ZUNIQUEIDENTIFIER)
	// ZLINKEDBY and ZLINKINGTO should be resolved to note UUIDs via JOIN
	require.NotNil(t, bl.ZLINKEDBY)
	assert.Equal(t, "note-uuid-2", *bl.ZLINKEDBY, "linked_by should resolve to note-uuid-2")
	require.NotNil(t, bl.ZLINKINGTO)
	assert.Equal(t, "note-uuid-1", *bl.ZLINKINGTO, "linking_to should resolve to note-uuid-1")
	require.NotNil(t, bl.ZTITLE)
	assert.Equal(t, "First Note", *bl.ZTITLE)
	require.NotNil(t, bl.ZLOCATION)
	assert.Equal(t, int64(42), *bl.ZLOCATION)
}

func TestBacklinks_Delta(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	bls, err := bearDB.Backlinks(ctx, modDateRecent)
	require.NoError(t, err)
	assert.Len(t, bls, 1)
}

func TestNoteTags_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.NoteTags(ctx)
	require.NoError(t, err)
	assert.Len(t, pairs, 4) // note1->tag1, note1->tag2, note2->tag1, note3->tag3

	// Verify UUID resolution
	foundPairs := map[string]bool{}
	for _, p := range pairs {
		foundPairs[p.NoteUUID+":"+p.TagUUID] = true
	}

	assert.True(t, foundPairs["note-uuid-1:tag-uuid-1"])
	assert.True(t, foundPairs["note-uuid-1:tag-uuid-2"])
	assert.True(t, foundPairs["note-uuid-2:tag-uuid-1"])
	assert.True(t, foundPairs["note-uuid-3:tag-uuid-3"])
}

func TestPinnedNoteTags_All(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.PinnedNoteTags(ctx)
	require.NoError(t, err)
	assert.Len(t, pairs, 1)
	assert.Equal(t, "note-uuid-1", pairs[0].NoteUUID)
	assert.Equal(t, "tag-uuid-1", pairs[0].TagUUID)
}

func TestNoteTagsForNotes(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.NoteTagsForNotes(ctx, []string{"note-uuid-1"})
	require.NoError(t, err)
	assert.Len(t, pairs, 2) // note1 has tag1 and tag2

	for _, p := range pairs {
		assert.Equal(t, "note-uuid-1", p.NoteUUID)
	}
}

func TestNoteTagsForNotes_Empty(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.NoteTagsForNotes(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, pairs)
}

func TestNoteTagsForNotes_MultipleNotes(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.NoteTagsForNotes(ctx, []string{"note-uuid-1", "note-uuid-3"})
	require.NoError(t, err)
	assert.Len(t, pairs, 3) // note1: tag1+tag2, note3: tag3
}

func TestPinnedNoteTagsForNotes(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	pairs, err := bearDB.PinnedNoteTagsForNotes(ctx, []string{"note-uuid-1"})
	require.NoError(t, err)
	assert.Len(t, pairs, 1)

	// Note 2 has no pinned tags
	pairs2, err := bearDB.PinnedNoteTagsForNotes(ctx, []string{"note-uuid-2"})
	require.NoError(t, err)
	assert.Empty(t, pairs2)
}

func TestAllNoteUUIDs(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	uuids, err := bearDB.AllNoteUUIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, uuids, 5) // note 6 has NULL UUID, excluded

	assert.Contains(t, uuids, "note-uuid-1")
	assert.Contains(t, uuids, "note-uuid-2")
	assert.Contains(t, uuids, "note-uuid-3")
	assert.Contains(t, uuids, "note-uuid-enc")
	assert.Contains(t, uuids, "note-uuid-trash")
}

func TestAllTagUUIDs(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	uuids, err := bearDB.AllTagUUIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, uuids, 3)
	assert.Contains(t, uuids, "tag-uuid-1")
	assert.Contains(t, uuids, "tag-uuid-2")
	assert.Contains(t, uuids, "tag-uuid-3")
}

func TestAllAttachmentUUIDs(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	uuids, err := bearDB.AllAttachmentUUIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, uuids, 2)
	assert.Contains(t, uuids, "att-uuid-1")
	assert.Contains(t, uuids, "att-uuid-2")
}

func TestAllBacklinkUUIDs(t *testing.T) {
	bearDB := setupTestBearDB(t)
	ctx := context.Background()

	uuids, err := bearDB.AllBacklinkUUIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, uuids, 1)
	assert.Contains(t, uuids, "bl-uuid-1")
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := beardb.New("/nonexistent/path/bear.sqlite")
	require.Error(t, err)
}

func TestNew_NotSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notdb.txt")

	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))

	_, err := beardb.New(path)
	require.Error(t, err)
}
