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
