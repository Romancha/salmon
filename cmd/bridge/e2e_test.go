package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/api"
	"github.com/romancha/bear-sync/internal/beardb"
	"github.com/romancha/bear-sync/internal/hubclient"
	"github.com/romancha/bear-sync/internal/models"
	"github.com/romancha/bear-sync/internal/store"

	_ "modernc.org/sqlite"
)

const (
	coreDataEpochOffsetE2E = 978307200
	firstNoteTitle         = "First Note"
)

func coreDataDateE2E(unixSec int64) float64 {
	return float64(unixSec - coreDataEpochOffsetE2E)
}

// e2eEnv holds all components wired together for E2E testing.
type e2eEnv struct {
	t              *testing.T
	bearDBPath     string
	bearDB         beardb.BearDB
	hubStore       store.Store
	apiServer      *httptest.Server
	hubClient      hubclient.HubClient
	bridge         *Bridge
	statePath      string
	attachmentsDir string
	bearDataDir    string
	openclawToken  string
	bridgeToken    string
}

func setupE2E(t *testing.T) *e2eEnv {
	t.Helper()

	tmpDir := t.TempDir()

	// Create Bear test SQLite.
	bearDBPath := filepath.Join(tmpDir, "bear.sqlite")
	createBearTestDB(t, bearDBPath)

	bearDB, err := beardb.New(bearDBPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bearDB.Close()) })

	// Create hub SQLite store.
	hubDBPath := filepath.Join(tmpDir, "hub.db")
	hubStore, err := store.NewSQLiteStore(hubDBPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, hubStore.Close()) })

	attachmentsDir := filepath.Join(tmpDir, "attachments")
	require.NoError(t, os.MkdirAll(attachmentsDir, 0o750))

	openclawToken := "test-openclaw-token"
	bridgeToken := "test-bridge-token"

	// Create API server.
	srv := api.NewServer(hubStore, openclawToken, bridgeToken, attachmentsDir)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Create hub client.
	hubCli := hubclient.NewHTTPClient(ts.URL, bridgeToken, testLogger())

	statePath := filepath.Join(tmpDir, "state.json")
	bearDataDir := filepath.Join(tmpDir, "bear-data")
	require.NoError(t, os.MkdirAll(bearDataDir, 0o750))

	bridge := NewBridge(bearDB, hubCli, nil, "test-bear-token", statePath, bearDataDir, testLogger())
	bridge.sleepFn = func(_ time.Duration) {}

	return &e2eEnv{
		t:              t,
		bearDBPath:     bearDBPath,
		bearDB:         bearDB,
		hubStore:       hubStore,
		apiServer:      ts,
		hubClient:      hubCli,
		bridge:         bridge,
		statePath:      statePath,
		attachmentsDir: attachmentsDir,
		bearDataDir:    bearDataDir,
		openclawToken:  openclawToken,
		bridgeToken:    bridgeToken,
	}
}

// createBearTestDB creates a Bear SQLite database with Core Data format test fixtures.
func createBearTestDB(t *testing.T, dbPath string) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	ctx := context.Background()

	// Schema matches Bear's Core Data SQLite format.
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

	// Test dates.
	createDate := coreDataDateE2E(1704024000)    // 2024-01-01
	modDate := coreDataDateE2E(1705320000)        // 2024-01-15 12:00 UTC

	// Insert notes: 3 regular + 1 encrypted + 1 trashed.
	noteInserts := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT,
				ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED, ZPINNED, ZHASIMAGES, ZVERSION,
				ZCREATIONDATE, ZMODIFICATIONDATE, ZLASTEDITINGDEVICE)
			VALUES (1, 'bear-note-1', 'First Note', 'sub1', '# First Note\nHello world',
				0, 0, 0, 1, 1, 3, ?, ?, 'MacBook')`,
			[]any{createDate, modDate},
		},
		{
			`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZSUBTITLE, ZTEXT,
				ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED, ZPINNED, ZVERSION,
				ZCREATIONDATE, ZMODIFICATIONDATE)
			VALUES (2, 'bear-note-2', 'Second Note', 'sub2', '# Second Note\nContent here',
				0, 0, 0, 0, 1, ?, ?)`,
			[]any{createDate, modDate},
		},
		{
			`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT,
				ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED, ZPINNED, ZVERSION,
				ZCREATIONDATE, ZMODIFICATIONDATE)
			VALUES (3, 'bear-note-3', 'Third Note', '# Third Note\nBacklink target',
				0, 0, 0, 0, 1, ?, ?)`,
			[]any{createDate, modDate},
		},
		{
			`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT,
				ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED, ZVERSION,
				ZCREATIONDATE, ZMODIFICATIONDATE, ZENCRYPTEDDATA, ZENCRYPTIONUNIQUEIDENTIFIER)
			VALUES (4, 'bear-note-enc', 'Encrypted Note', NULL,
				1, 0, 0, 1, ?, ?, X'DEADBEEF', 'enc-id-1')`,
			[]any{createDate, modDate},
		},
		{
			`INSERT INTO ZSFNOTE (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZTEXT,
				ZENCRYPTED, ZTRASHED, ZPERMANENTLYDELETED, ZVERSION,
				ZCREATIONDATE, ZMODIFICATIONDATE, ZTRASHEDDATE)
			VALUES (5, 'bear-note-trash', 'Trashed Note', '# Trashed',
				0, 1, 0, 1, ?, ?, ?)`,
			[]any{createDate, modDate, modDate},
		},
	}

	for _, ni := range noteInserts {
		_, err := db.ExecContext(ctx, ni.query, ni.args...)
		require.NoError(t, err)
	}

	// Insert tags: root + child hierarchy + standalone.
	tagInserts := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION, ZMODIFICATIONDATE)
			VALUES (1, 'bear-tag-work', 'work', 0, 1, 1, ?)`,
			[]any{modDate},
		},
		{
			`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION, ZMODIFICATIONDATE)
			VALUES (2, 'bear-tag-work-projects', 'work/projects', 0, 0, 1, ?)`,
			[]any{modDate},
		},
		{
			`INSERT INTO ZSFNOTETAG (Z_PK, ZUNIQUEIDENTIFIER, ZTITLE, ZPINNED, ZISROOT, ZVERSION, ZMODIFICATIONDATE)
			VALUES (3, 'bear-tag-personal', 'personal', 1, 1, 2, ?)`,
			[]any{modDate},
		},
	}

	for _, ti := range tagInserts {
		_, err := db.ExecContext(ctx, ti.query, ti.args...)
		require.NoError(t, err)
	}

	// Insert attachments: 1 image + 1 file.
	_, err = db.ExecContext(ctx,
		`INSERT INTO ZSFNOTEFILE (Z_PK, Z_ENT, ZUNIQUEIDENTIFIER, ZNOTE, ZFILENAME,
			ZNORMALIZEDFILEEXTENSION, ZFILESIZE, ZWIDTH, ZHEIGHT, ZVERSION,
			ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (1, 9, 'bear-att-img', 1, 'photo.jpg', 'jpg', 102400, 1920, 1080, 1, ?, ?)`,
		createDate, modDate)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx,
		`INSERT INTO ZSFNOTEFILE (Z_PK, Z_ENT, ZUNIQUEIDENTIFIER, ZNOTE, ZFILENAME,
			ZNORMALIZEDFILEEXTENSION, ZFILESIZE, ZVERSION,
			ZCREATIONDATE, ZMODIFICATIONDATE)
		VALUES (2, 8, 'bear-att-file', 2, 'document.pdf', 'pdf', 51200, 1, ?, ?)`,
		createDate, modDate)
	require.NoError(t, err)

	// Insert backlink: note-2 links to note-3.
	_, err = db.ExecContext(ctx,
		`INSERT INTO ZSFNOTEBACKLINK (Z_PK, ZUNIQUEIDENTIFIER, ZLINKEDBY, ZLINKINGTO,
			ZTITLE, ZLOCATION, ZVERSION, ZMODIFICATIONDATE)
		VALUES (1, 'bear-bl-1', 2, 3, 'Third Note', 42, 1, ?)`, modDate)
	require.NoError(t, err)

	// Junction tables: note1 -> tag:work, tag:work/projects; note2 -> tag:work; note3 -> tag:personal.
	junctions := []string{
		"INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (1, 1)",
		"INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (1, 2)",
		"INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (2, 1)",
		"INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (3, 3)",
	}
	for _, q := range junctions {
		_, err := db.ExecContext(ctx, q)
		require.NoError(t, err)
	}

	// Pinned junction: note1 pinned in tag:work.
	_, err = db.ExecContext(ctx,
		"INSERT INTO Z_5PINNEDINTAGS (Z_5PINNEDNOTES, Z_13PINNEDINTAGS) VALUES (1, 1)")
	require.NoError(t, err)
}

// httpResult holds a response from an HTTP call with body already read and closed.
type httpResult struct {
	StatusCode int
	Body       []byte
}

// openclawGet performs an authenticated GET request as openclaw.
func (e *e2eEnv) openclawGet(path string) httpResult {
	e.t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.apiServer.URL+path, http.NoBody)
	require.NoError(e.t, err)
	req.Header.Set("Authorization", "Bearer "+e.openclawToken)

	return e.doHTTP(req)
}

// openclawDo performs an authenticated request as openclaw.
func (e *e2eEnv) openclawDo(method, path string, body any, idempotencyKey string) httpResult {
	e.t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(e.t, err)
		bodyReader = strings.NewReader(string(data))
	} else {
		bodyReader = http.NoBody
	}

	req, err := http.NewRequestWithContext(context.Background(), method, e.apiServer.URL+path, bodyReader)
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")

	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	return e.doHTTP(req)
}

func (e *e2eEnv) doHTTP(req *http.Request) httpResult {
	e.t.Helper()
	req.Header.Set("Authorization", "Bearer "+e.openclawToken)

	resp, err := e.apiServer.Client().Do(req) //nolint:gosec // test httptest server
	require.NoError(e.t, err)
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(e.t, err)

	return httpResult{StatusCode: resp.StatusCode, Body: data}
}

func decodeJSON[T any](t *testing.T, r httpResult) T {
	t.Helper()

	var v T
	require.NoError(t, json.Unmarshal(r.Body, &v))

	return v
}

// --- E2E Tests ---

// TestE2E_ReadFlow tests the full read path:
// bridge reads Bear SQLite -> push to hub -> openclaw API reads back.
func TestE2E_ReadFlow(t *testing.T) {
	env := setupE2E(t)

	// Run bridge initial sync (reads Bear -> pushes to hub).
	err := env.bridge.Run(context.Background())
	require.NoError(t, err)

	// Verify notes via openclaw API.
	resp := env.openclawGet("/api/notes?limit=50")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	notes := decodeJSON[[]models.Note](t, resp)
	assert.Len(t, notes, 5) // 3 regular + 1 encrypted + 1 trashed

	// Verify specific note with full details.
	var noteID string
	for _, n := range notes {
		if n.Title == firstNoteTitle {
			noteID = n.ID
			break
		}
	}
	require.NotEmpty(t, noteID, "First Note should exist in hub")

	resp = env.openclawGet("/api/notes/" + noteID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	note := decodeJSON[models.Note](t, resp)
	assert.Equal(t, firstNoteTitle, note.Title)
	assert.Contains(t, note.Body, "Hello world")
	assert.Equal(t, 1, note.Pinned)
	require.NotNil(t, note.BearID)
	assert.Equal(t, "bear-note-1", *note.BearID)

	// Note should have tags.
	assert.Len(t, note.Tags, 2) // work + work/projects
	tagTitles := make([]string, len(note.Tags))
	for i, tg := range note.Tags {
		tagTitles[i] = tg.Title
	}
	assert.Contains(t, tagTitles, "work")
	assert.Contains(t, tagTitles, "work/projects")

	// Verify tags via API.
	resp = env.openclawGet("/api/tags")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	tags := decodeJSON[[]models.Tag](t, resp)
	assert.Len(t, tags, 3) // work, work/projects, personal

	// Verify FTS5 search works.
	resp = env.openclawGet("/api/notes/search?q=" + strings.ReplaceAll(firstNoteTitle, " ", "+"))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	searchResults := decodeJSON[[]models.Note](t, resp)
	require.GreaterOrEqual(t, len(searchResults), 1)
	assert.Equal(t, firstNoteTitle, searchResults[0].Title)

	// Verify backlinks.
	var note3ID string
	for _, n := range notes {
		if n.Title == "Third Note" {
			note3ID = n.ID
			break
		}
	}
	require.NotEmpty(t, note3ID)

	resp = env.openclawGet(fmt.Sprintf("/api/notes/%s/backlinks", note3ID))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	backlinks := decodeJSON[[]models.Backlink](t, resp)
	assert.Len(t, backlinks, 1)
	assert.Equal(t, "Third Note", backlinks[0].Title)

	// Verify encrypted note is present.
	var encNote *models.Note
	for _, n := range notes {
		if n.Title == "Encrypted Note" {
			n := n
			encNote = &n
			break
		}
	}
	require.NotNil(t, encNote)
	assert.Equal(t, 1, encNote.Encrypted)

	// Verify trashed note.
	var trashedNote *models.Note
	for _, n := range notes {
		if n.Title == "Trashed Note" {
			n := n
			trashedNote = &n
			break
		}
	}
	require.NotNil(t, trashedNote)
	assert.Equal(t, 1, trashedNote.Trashed)

	// Verify state file was created.
	state, err := loadState(env.statePath)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Greater(t, state.LastSyncAt, float64(0))
	assert.Len(t, state.KnownNoteIDs, 5)
	assert.Len(t, state.KnownTagIDs, 3)
}

// TestE2E_WriteFlow tests the full write path:
// openclaw POST/PUT -> write_queue -> bridge lease -> mock xcall ack -> verify.
func TestE2E_WriteFlow(t *testing.T) {
	env := setupE2E(t)

	// First do initial sync to populate hub.
	require.NoError(t, env.bridge.Run(context.Background()))

	// Create a note via openclaw API.
	createResp := env.openclawDo(http.MethodPost, "/api/notes", map[string]string{
		"title": "New Note From Openclaw",
		"body":  "# New Note\nCreated via API",
	}, "idem-create-1")
	assert.Equal(t, http.StatusCreated, createResp.StatusCode)
	createdNote := decodeJSON[models.Note](t, createResp)
	assert.Equal(t, "New Note From Openclaw", createdNote.Title)
	assert.Equal(t, "pending_to_bear", createdNote.SyncStatus)

	// Update a note via openclaw API — find an existing note.
	listResp := env.openclawGet("/api/notes?limit=50")
	allNotes := decodeJSON[[]models.Note](t, listResp)
	var existingNoteID string
	for _, n := range allNotes {
		if n.Title == firstNoteTitle {
			existingNoteID = n.ID
			break
		}
	}
	require.NotEmpty(t, existingNoteID)

	updateResp := env.openclawDo(http.MethodPut, "/api/notes/"+existingNoteID, map[string]string{
		"body": "# Updated First Note\nUpdated body from openclaw",
	}, "idem-update-1")
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Add tag via openclaw API.
	addTagResp := env.openclawDo(http.MethodPost,
		fmt.Sprintf("/api/notes/%s/tags", existingNoteID),
		map[string]string{"tag": "new-tag"}, "idem-tag-1")
	assert.Equal(t, http.StatusCreated, addTagResp.StatusCode)

	// Trash a note via openclaw API.
	var note2ID string
	for _, n := range allNotes {
		if n.Title == "Second Note" {
			note2ID = n.ID
			break
		}
	}
	require.NotEmpty(t, note2ID)

	trashResp := env.openclawDo(http.MethodDelete, "/api/notes/"+note2ID, nil, "idem-trash-1")
	assert.Equal(t, http.StatusOK, trashResp.StatusCode)

	// Lease queue items via hub API (as bridge would).
	items, err := env.hubStore.LeaseQueueItems(context.Background(), "bridge-test", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items, 4) // create + update + add_tag + trash

	// Verify actions.
	actions := make(map[string]bool)
	for _, item := range items {
		actions[item.Action] = true
	}
	assert.True(t, actions["create"])
	assert.True(t, actions["update"])
	assert.True(t, actions["add_tag"])
	assert.True(t, actions["trash"])

	// Simulate bridge ack with bear_id for the created note.
	ackItems := make([]models.SyncAckItem, 0, len(items))
	for _, item := range items {
		ack := models.SyncAckItem{
			QueueID:        item.ID,
			IdempotencyKey: item.IdempotencyKey,
			Status:         "applied",
		}
		if item.Action == "create" {
			ack.BearID = "new-bear-uuid-from-xcall"
		}
		ackItems = append(ackItems, ack)
	}

	require.NoError(t, env.hubStore.AckQueueItems(context.Background(), ackItems))

	// Verify the created note now has a bear_id.
	createdNoteAfterAck, err := env.hubStore.GetNote(context.Background(), createdNote.ID)
	require.NoError(t, err)
	require.NotNil(t, createdNoteAfterAck)
	require.NotNil(t, createdNoteAfterAck.BearID)
	assert.Equal(t, "new-bear-uuid-from-xcall", *createdNoteAfterAck.BearID)
	assert.Equal(t, "synced", createdNoteAfterAck.SyncStatus)
}

// TestE2E_Idempotency tests that repeated HTTP requests with the same Idempotency-Key
// don't create duplicates in the write queue.
func TestE2E_Idempotency(t *testing.T) {
	env := setupE2E(t)
	require.NoError(t, env.bridge.Run(context.Background()))

	// Create a note with the same idempotency key twice.
	body := map[string]string{
		"title": "Idempotent Note",
		"body":  "# Test",
	}

	resp1 := env.openclawDo(http.MethodPost, "/api/notes", body, "idem-same-key")
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)
	note1 := decodeJSON[models.Note](t, resp1)

	resp2 := env.openclawDo(http.MethodPost, "/api/notes", body, "idem-same-key")
	// Second request should also succeed (returns existing).
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	// Only 1 item in write queue for this idempotency key.
	items, err := env.hubStore.LeaseQueueItems(context.Background(), "test", 5*time.Minute)
	require.NoError(t, err)

	createCount := 0
	for _, item := range items {
		if item.Action == "create" && item.NoteID == note1.ID {
			createCount++
		}
	}
	assert.Equal(t, 1, createCount, "only one create item should exist for the same idempotency key")
}

// TestE2E_CrashRecovery tests that items with expired leases are re-picked up.
// Simulates: apply without ack -> lease expiry -> re-pickup.
func TestE2E_CrashRecovery(t *testing.T) {
	env := setupE2E(t)
	require.NoError(t, env.bridge.Run(context.Background()))

	// Create a note via openclaw to enqueue a write.
	resp := env.openclawDo(http.MethodPost, "/api/notes", map[string]string{
		"title": "Crash Recovery Note",
		"body":  "# Test",
	}, "idem-crash-1")
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Lease the item normally.
	items, err := env.hubStore.LeaseQueueItems(context.Background(), "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "processing", items[0].Status)

	// Simulate crash: no ack. Force-expire the lease by setting lease_until to the past.
	sqliteStore := env.hubStore.(*store.SQLiteStore)
	pastTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	_, err = sqliteStore.DB().ExecContext(context.Background(),
		"UPDATE write_queue SET lease_until = ? WHERE id = ?", pastTime, items[0].ID)
	require.NoError(t, err)

	// Another bridge instance should be able to pick up the item after lease expiry.
	items2, err := env.hubStore.LeaseQueueItems(context.Background(), "bridge-2", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items2, 1)
	assert.Equal(t, items[0].ID, items2[0].ID, "same item should be re-leased after expiry")
	assert.Equal(t, "bridge-2", items2[0].ProcessingBy)
}

// TestE2E_JunctionTableFullScan tests that junction table changes without note modification
// are detected during the full-scan cycle.
func TestE2E_JunctionTableFullScan(t *testing.T) {
	env := setupE2E(t)

	// Run initial sync.
	require.NoError(t, env.bridge.Run(context.Background()))

	// Read the state to know the known pairs.
	state, err := loadState(env.statePath)
	require.NoError(t, err)
	require.NotNil(t, state)
	initialPairs := len(state.KnownNoteTagPairs)

	// Modify the Bear SQLite directly — add a tag association without updating ZMODIFICATIONDATE.
	bearRawDB, err := sql.Open("sqlite", env.bearDBPath)
	require.NoError(t, err)
	defer func() { _ = bearRawDB.Close() }()

	// Add note-3 to tag:work (Z_PK=1) — note-3 already has tag:personal.
	_, err = bearRawDB.ExecContext(context.Background(),
		"INSERT INTO Z_5TAGS (Z_5NOTES, Z_13TAGS) VALUES (3, 1)")
	require.NoError(t, err)

	// Set junction counter to trigger full scan (multiple of 12).
	state.JunctionFullScanCounter = 12
	require.NoError(t, saveState(env.statePath, state))

	// Re-open bear DB to pick up changes.
	_ = env.bearDB.Close()
	newBearDB, err := beardb.New(env.bearDBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = newBearDB.Close() })

	env.bridge.db = newBearDB

	// Run delta sync — full scan should detect the new junction.
	require.NoError(t, env.bridge.Run(context.Background()))

	// Verify updated state has more pairs.
	updatedState, err := loadState(env.statePath)
	require.NoError(t, err)
	assert.Greater(t, len(updatedState.KnownNoteTagPairs), initialPairs)
	assert.Equal(t, 13, updatedState.JunctionFullScanCounter)

	// Verify via API: note-3 should now have 2 tags.
	allNotesResp := env.openclawGet("/api/notes?limit=50")
	allNotes := decodeJSON[[]models.Note](t, allNotesResp)
	for _, n := range allNotes {
		if n.BearID != nil && *n.BearID == "bear-note-3" {
			resp := env.openclawGet("/api/notes/" + n.ID)
			note := decodeJSON[models.Note](t, resp)
			assert.Len(t, note.Tags, 2, "note-3 should have 2 tags after full scan sync")
			break
		}
	}
}

// TestE2E_EncryptedNoteRestrictions tests that PUT/DELETE on encrypted notes returns 403.
func TestE2E_EncryptedNoteRestrictions(t *testing.T) {
	env := setupE2E(t)
	require.NoError(t, env.bridge.Run(context.Background()))

	// Find the encrypted note.
	resp := env.openclawGet("/api/notes?limit=50")
	notes := decodeJSON[[]models.Note](t, resp)
	var encNoteID string
	for _, n := range notes {
		if n.Encrypted == 1 {
			encNoteID = n.ID
			break
		}
	}
	require.NotEmpty(t, encNoteID, "encrypted note should exist")

	// Try to update encrypted note — should be 403.
	updateResp := env.openclawDo(http.MethodPut, "/api/notes/"+encNoteID, map[string]string{
		"body": "try to update encrypted",
	}, "idem-enc-update")
	assert.Equal(t, http.StatusForbidden, updateResp.StatusCode)

	// Try to delete (trash) encrypted note — should be 403.
	deleteResp := env.openclawDo(http.MethodDelete, "/api/notes/"+encNoteID, nil, "idem-enc-delete")
	assert.Equal(t, http.StatusForbidden, deleteResp.StatusCode)

	// Try to add tag to encrypted note — should be 403.
	tagResp := env.openclawDo(http.MethodPost,
		fmt.Sprintf("/api/notes/%s/tags", encNoteID),
		map[string]string{"tag": "new-tag"}, "idem-enc-tag")
	assert.Equal(t, http.StatusForbidden, tagResp.StatusCode)

	// GET should still work.
	getResp := env.openclawGet("/api/notes/" + encNoteID)
	assert.Equal(t, http.StatusOK, getResp.StatusCode)
}

// TestE2E_ConflictDetection tests conflict detection when openclaw updates a note
// and Bear pushes a new modified_at simultaneously.
func TestE2E_ConflictDetection(t *testing.T) {
	env := setupE2E(t)
	require.NoError(t, env.bridge.Run(context.Background()))

	// Find a note to conflict on.
	resp := env.openclawGet("/api/notes?limit=50")
	notes := decodeJSON[[]models.Note](t, resp)
	var targetNoteID string
	for _, n := range notes {
		if n.Title == firstNoteTitle {
			targetNoteID = n.ID
			break
		}
	}
	require.NotEmpty(t, targetNoteID)

	// Openclaw updates the note — sets sync_status=pending_to_bear.
	updateResp := env.openclawDo(http.MethodPut, "/api/notes/"+targetNoteID, map[string]string{
		"body": "# Updated by openclaw",
	}, "idem-conflict-update")
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Verify note is pending_to_bear.
	noteAfterUpdate, err := env.hubStore.GetNote(context.Background(), targetNoteID)
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", noteAfterUpdate.SyncStatus)

	// Now simulate Bear push with a DIFFERENT modified_at — conflict!
	// The bridge would push a note with the same bear_id but different modified_at.
	pushReq := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     strPtr("bear-note-1"),
				Title:      firstNoteTitle,
				Body:       "# First Note\nEdited in Bear directly",
				ModifiedAt: "2025-01-20T10:00:00Z", // Different from what openclaw set
				Version:    4,
			},
		},
	}

	err = env.hubClient.SyncPush(context.Background(), pushReq)
	require.NoError(t, err)

	// Verify the note is now in conflict state.
	noteAfterConflict, err := env.hubStore.GetNote(context.Background(), targetNoteID)
	require.NoError(t, err)
	assert.Equal(t, "conflict", noteAfterConflict.SyncStatus)

	// Verify sync status endpoint is accessible.
	_ = env.openclawGet("/api/sync/status")

	// Check conflict count directly via store.
	conflictCount, err := env.hubStore.CountConflicts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, conflictCount)

	conflictIDs, err := env.hubStore.ListConflictNoteIDs(context.Background())
	require.NoError(t, err)
	assert.Contains(t, conflictIDs, targetNoteID)
}

// Also generate the testdata Bear SQLite fixture (for use by other tests/packages).
func TestGenerateTestdataBearSQLite(t *testing.T) {
	if os.Getenv("GENERATE_TESTDATA") == "" {
		t.Skip("set GENERATE_TESTDATA=1 to regenerate testdata/bear.sqlite")
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	// Navigate to project root.
	root := filepath.Join(wd, "..", "..")
	dbPath := filepath.Join(root, "testdata", "bear.sqlite")

	// Remove existing.
	_ = os.Remove(dbPath)

	createBearTestDB(t, dbPath)
	t.Logf("generated %s", dbPath)
}

