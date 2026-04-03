package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttachment_ToInfo(t *testing.T) {
	a := Attachment{
		ID:       "att-1",
		NoteID:   "note-1",
		Type:     "image",
		Filename: "photo.jpg",
		FileSize: 1024,
		Width:    800,
		Height:   600,
		BearRaw:  "should-not-appear",
		FilePath: "/secret/path",
	}

	info := a.ToInfo()

	assert.Equal(t, "att-1", info.ID)
	assert.Equal(t, "image", info.Type)
	assert.Equal(t, "photo.jpg", info.Filename)
	assert.Equal(t, int64(1024), info.FileSize)
	assert.Equal(t, 800, info.Width)
	assert.Equal(t, 600, info.Height)
}

func TestAttachmentsToInfo(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		assert.Nil(t, AttachmentsToInfo(nil))
	})

	t.Run("empty slice", func(t *testing.T) {
		assert.Nil(t, AttachmentsToInfo([]Attachment{}))
	})

	t.Run("multiple attachments", func(t *testing.T) {
		attachments := []Attachment{
			{ID: "a1", Type: "image", Filename: "pic.png", FileSize: 100, Width: 10, Height: 20},
			{ID: "a2", Type: "file", Filename: "doc.pdf", FileSize: 200},
		}

		infos := AttachmentsToInfo(attachments)

		assert.Len(t, infos, 2)
		assert.Equal(t, "a1", infos[0].ID)
		assert.Equal(t, "a2", infos[1].ID)
		assert.Equal(t, "doc.pdf", infos[1].Filename)
	})
}

func TestAttachment_ToMeta(t *testing.T) {
	a := Attachment{
		ID:                  "att-1",
		NoteID:              "note-1",
		Type:                "image",
		Filename:            "photo.jpg",
		NormalizedExtension: "jpg",
		FileSize:            1024,
		Width:               800,
		Height:              600,
		CreatedAt:           "2025-01-15T10:00:00Z",
		ModifiedAt:          "2025-01-15T11:00:00Z",
		BearRaw:             "should-not-appear",
		FilePath:            "/secret/path",
		EncryptionID:        "enc-1",
	}

	meta := a.ToMeta()

	assert.Equal(t, "att-1", meta.ID)
	assert.Equal(t, "image", meta.Type)
	assert.Equal(t, "photo.jpg", meta.Filename)
	assert.Equal(t, "jpg", meta.NormalizedExtension)
	assert.Equal(t, int64(1024), meta.FileSize)
	assert.Equal(t, 800, meta.Width)
	assert.Equal(t, 600, meta.Height)
	assert.Equal(t, "2025-01-15T10:00:00Z", meta.CreatedAt)
	assert.Equal(t, "2025-01-15T11:00:00Z", meta.ModifiedAt)
}

func TestAttachmentsToMeta(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		assert.Nil(t, AttachmentsToMeta(nil))
	})

	t.Run("empty slice", func(t *testing.T) {
		assert.Nil(t, AttachmentsToMeta([]Attachment{}))
	})

	t.Run("multiple attachments", func(t *testing.T) {
		attachments := []Attachment{
			{ID: "a1", Type: "image", Filename: "pic.png", NormalizedExtension: "png", CreatedAt: "2025-01-01"},
			{ID: "a2", Type: "file", Filename: "doc.pdf", NormalizedExtension: "pdf", ModifiedAt: "2025-02-01"},
		}

		metas := AttachmentsToMeta(attachments)

		assert.Len(t, metas, 2)
		assert.Equal(t, "a1", metas[0].ID)
		assert.Equal(t, "png", metas[0].NormalizedExtension)
		assert.Equal(t, "a2", metas[1].ID)
		assert.Equal(t, "pdf", metas[1].NormalizedExtension)
	})
}
