package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNote_StripInternal(t *testing.T) {
	n := Note{
		ID:            "n1",
		Title:         "Test Note",
		Body:          "body",
		SyncStatus:    "synced",
		BearRaw:       `{"raw":"data"}`,
		EncryptedData: []byte("secret"),
		HubModifiedAt: "2025-01-01T00:00:00Z",
		Tags: []Tag{
			{ID: "t1", Title: "work", BearRaw: "tag-raw"},
		},
		Backlinks: []Backlink{
			{ID: "b1", LinkedByID: "n2", BearRaw: "bl-raw"},
		},
	}

	n.StripInternal()

	// Internal fields stripped.
	assert.Empty(t, n.BearRaw)
	assert.Nil(t, n.EncryptedData)
	assert.Empty(t, n.HubModifiedAt)
	assert.Empty(t, n.Tags[0].BearRaw)
	assert.Empty(t, n.Backlinks[0].BearRaw)

	// Non-internal fields preserved.
	assert.Equal(t, "n1", n.ID)
	assert.Equal(t, "Test Note", n.Title)
	assert.Equal(t, "body", n.Body)
	assert.Equal(t, "synced", n.SyncStatus)
	assert.Equal(t, "t1", n.Tags[0].ID)
	assert.Equal(t, "work", n.Tags[0].Title)
	assert.Equal(t, "b1", n.Backlinks[0].ID)
}

func TestNote_StripInternal_EmptyJoins(t *testing.T) {
	n := Note{ID: "n1", BearRaw: "raw"}

	n.StripInternal()

	assert.Empty(t, n.BearRaw)
	assert.Empty(t, n.Tags)
	assert.Empty(t, n.Backlinks)
}
