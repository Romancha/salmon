package mapper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T {
	return &v
}

func TestGenerateID(t *testing.T) {
	id, err := GenerateID()
	require.NoError(t, err)
	assert.Len(t, id, 32)

	id2, err := GenerateID()
	require.NoError(t, err)
	assert.NotEqual(t, id, id2)
}

func TestConvertCoreDataDate(t *testing.T) {
	tests := []struct {
		name     string
		input    *float64
		expected string
	}{
		{
			name:     "nil returns empty",
			input:    nil,
			expected: "",
		},
		{
			name:     "known Core Data timestamp",
			input:    ptr(726842600.0),
			expected: "2024-01-13T12:43:20Z",
		},
		{
			name:     "2001-01-01 00:00:00 UTC (Core Data epoch zero)",
			input:    ptr(0.0),
			expected: "2001-01-01T00:00:00Z",
		},
		{
			name:     "negative value before 2001",
			input:    ptr(-86400.0), // one day before Core Data epoch
			expected: "2000-12-31T00:00:00Z",
		},
		{
			name:     "fractional seconds",
			input:    ptr(726842600.5),
			expected: "2024-01-13T12:43:20Z", // sub-second truncated in RFC3339
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertCoreDataDate(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertAttachmentType(t *testing.T) {
	assert.Equal(t, "file", ConvertAttachmentType(8))
	assert.Equal(t, "image", ConvertAttachmentType(9))
	assert.Equal(t, "video", ConvertAttachmentType(10))
	assert.Equal(t, "file", ConvertAttachmentType(0))
	assert.Equal(t, "file", ConvertAttachmentType(99))
}

func TestMapBearNote(t *testing.T) {
	row := BearNoteRow{
		ZPK:                1,
		ZUNIQUEIDENTIFIER:  ptr("BEAR-UUID-123"),
		ZTITLE:             ptr("Test Note"),
		ZSUBTITLE:          ptr("A subtitle"),
		ZTEXT:              ptr("# Test Note\nBody text"),
		ZARCHIVED:          ptr(int64(0)),
		ZENCRYPTED:         ptr(int64(0)),
		ZHASFILES:          ptr(int64(1)),
		ZHASIMAGES:         ptr(int64(1)),
		ZHASSOURCECODE:     ptr(int64(0)),
		ZLOCKED:            ptr(int64(0)),
		ZPINNED:            ptr(int64(1)),
		ZSHOWNINTODAYWIDGET: ptr(int64(0)),
		ZTRASHED:           ptr(int64(0)),
		ZPERMANENTLYDELETED: ptr(int64(0)),
		ZSKIPSYNC:          ptr(int64(0)),
		ZTODOCOMPLETED:     ptr(int64(2)),
		ZTODOINCOMPLETED:   ptr(int64(3)),
		ZVERSION:           ptr(int64(5)),
		ZCREATIONDATE:      ptr(726842600.0),
		ZMODIFICATIONDATE:  ptr(726842700.0),
		ZLASTEDITINGDEVICE: ptr("MacBook Pro"),
	}

	note, err := MapBearNote(&row)
	require.NoError(t, err)

	assert.Len(t, note.ID, 32)
	assert.Equal(t, "BEAR-UUID-123", *note.BearID)
	assert.Equal(t, "Test Note", note.Title)
	assert.Equal(t, "A subtitle", note.Subtitle)
	assert.Equal(t, "# Test Note\nBody text", note.Body)
	assert.Equal(t, 0, note.Archived)
	assert.Equal(t, 0, note.Encrypted)
	assert.Equal(t, 1, note.HasFiles)
	assert.Equal(t, 1, note.HasImages)
	assert.Equal(t, 0, note.HasSourceCode)
	assert.Equal(t, 0, note.Locked)
	assert.Equal(t, 1, note.Pinned)
	assert.Equal(t, 0, note.ShownInToday)
	assert.Equal(t, 0, note.Trashed)
	assert.Equal(t, 0, note.PermanentlyDeleted)
	assert.Equal(t, 0, note.SkipSync)
	assert.Equal(t, 2, note.TodoCompleted)
	assert.Equal(t, 3, note.TodoIncompleted)
	assert.Equal(t, 5, note.Version)
	assert.NotEmpty(t, note.CreatedAt)
	assert.NotEmpty(t, note.ModifiedAt)
	assert.Empty(t, note.ArchivedAt)
	assert.Equal(t, "MacBook Pro", note.LastEditingDevice)
	assert.Equal(t, "synced", note.SyncStatus)
	assert.NotEmpty(t, note.BearRaw)

	// Verify bear_raw is valid JSON containing original fields
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(note.BearRaw), &raw))
	assert.Equal(t, float64(1), raw["ZPK"])
	assert.Equal(t, "BEAR-UUID-123", raw["ZUNIQUEIDENTIFIER"])
}

func TestMapBearNote_NullBearID(t *testing.T) {
	row := BearNoteRow{
		ZPK:    1,
		ZTITLE: ptr("Note without UUID"),
		ZTEXT:  ptr("body"),
	}

	note, err := MapBearNote(&row)
	require.NoError(t, err)

	assert.Nil(t, note.BearID)
	assert.Equal(t, "Note without UUID", note.Title)
	assert.Equal(t, "body", note.Body)
}

func TestMapBearNote_EncryptedNote(t *testing.T) {
	encData := []byte{0x01, 0x02, 0x03, 0x04}
	row := BearNoteRow{
		ZPK:                         1,
		ZUNIQUEIDENTIFIER:           ptr("ENC-UUID"),
		ZTITLE:                      ptr(""),
		ZTEXT:                       ptr(""),
		ZENCRYPTED:                  ptr(int64(1)),
		ZENCRYPTEDDATA:              encData,
		ZENCRYPTIONUNIQUEIDENTIFIER: ptr("enc-key-id"),
		ZENCRYPTIONDATE:             ptr(726842600.0),
	}

	note, err := MapBearNote(&row)
	require.NoError(t, err)

	assert.Equal(t, 1, note.Encrypted)
	assert.Equal(t, encData, note.EncryptedData)
	assert.Equal(t, "enc-key-id", note.EncryptionID)
	assert.NotEmpty(t, note.EncryptedAt)
}

func TestMapBearNote_AllNullFields(t *testing.T) {
	row := BearNoteRow{ZPK: 1}

	note, err := MapBearNote(&row)
	require.NoError(t, err)

	assert.Len(t, note.ID, 32)
	assert.Nil(t, note.BearID)
	assert.Empty(t, note.Title)
	assert.Empty(t, note.Body)
	assert.Equal(t, 0, note.Version)
	assert.Empty(t, note.CreatedAt)
	assert.Empty(t, note.ModifiedAt)
	assert.Equal(t, "synced", note.SyncStatus)
}

func TestMapBearTag(t *testing.T) {
	row := BearTagRow{
		ZPK:               1,
		ZUNIQUEIDENTIFIER: ptr("TAG-UUID-456"),
		ZTITLE:            ptr("08_Knowledge/IT"),
		ZPINNED:           ptr(int64(1)),
		ZISROOT:           ptr(int64(0)),
		ZHIDESUBTAGSNOTES: ptr(int64(0)),
		ZSORTING:          ptr(int64(2)),
		ZSORTINGDIRECTION: ptr(int64(1)),
		ZENCRYPTED:        ptr(int64(0)),
		ZVERSION:          ptr(int64(3)),
		ZMODIFICATIONDATE: ptr(726842600.0),
		ZPINNEDDATE:       ptr(726842500.0),
		ZTAGCON:           ptr("tag-con-value"),
	}

	tag, err := MapBearTag(&row)
	require.NoError(t, err)

	assert.Len(t, tag.ID, 32)
	assert.Equal(t, "TAG-UUID-456", *tag.BearID)
	assert.Equal(t, "08_Knowledge/IT", tag.Title)
	assert.Equal(t, 1, tag.Pinned)
	assert.Equal(t, 0, tag.IsRoot)
	assert.Equal(t, 2, tag.Sorting)
	assert.Equal(t, 1, tag.SortingDirection)
	assert.Equal(t, 0, tag.Encrypted)
	assert.Equal(t, 3, tag.Version)
	assert.NotEmpty(t, tag.ModifiedAt)
	assert.NotEmpty(t, tag.PinnedAt)
	assert.Empty(t, tag.PinnedNotesAt)
	assert.Equal(t, "tag-con-value", tag.TagCon)
	assert.NotEmpty(t, tag.BearRaw)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(tag.BearRaw), &raw))
	assert.Equal(t, "TAG-UUID-456", raw["ZUNIQUEIDENTIFIER"])
}

func TestMapBearTag_NullBearID(t *testing.T) {
	row := BearTagRow{
		ZPK:    1,
		ZTITLE: ptr("orphan-tag"),
	}

	tag, err := MapBearTag(&row)
	require.NoError(t, err)

	assert.Nil(t, tag.BearID)
	assert.Equal(t, "orphan-tag", tag.Title)
}

func TestMapBearAttachment(t *testing.T) {
	row := BearAttachmentRow{
		ZPK:                      1,
		ZENT:                     9, // image
		ZUNIQUEIDENTIFIER:        ptr("ATT-UUID-789"),
		ZNOTE:                    ptr("NOTE-UUID-123"),
		ZFILENAME:                ptr("photo.jpg"),
		ZNORMALIZEDFILEEXTENSION: ptr("jpg"),
		ZFILESIZE:                ptr(int64(1024000)),
		ZINDEX:                   ptr(int64(0)),
		ZWIDTH:                   ptr(int64(1920)),
		ZHEIGHT:                  ptr(int64(1080)),
		ZANIMATED:                ptr(int64(0)),
		ZDOWNLOADED:              ptr(int64(1)),
		ZENCRYPTED:               ptr(int64(0)),
		ZPERMANENTLYDELETED:      ptr(int64(0)),
		ZSKIPSYNC:                ptr(int64(0)),
		ZUNUSED:                  ptr(int64(0)),
		ZUPLOADED:                ptr(int64(1)),
		ZVERSION:                 ptr(int64(2)),
		ZCREATIONDATE:            ptr(726842600.0),
		ZMODIFICATIONDATE:        ptr(726842700.0),
	}

	att, err := MapBearAttachment(&row)
	require.NoError(t, err)

	assert.Len(t, att.ID, 32)
	assert.Equal(t, "ATT-UUID-789", *att.BearID)
	assert.Equal(t, "NOTE-UUID-123", att.NoteID)
	assert.Equal(t, "image", att.Type)
	assert.Equal(t, "photo.jpg", att.Filename)
	assert.Equal(t, "jpg", att.NormalizedExtension)
	assert.Equal(t, int64(1024000), att.FileSize)
	assert.Equal(t, 0, att.FileIndex)
	assert.Equal(t, 1920, att.Width)
	assert.Equal(t, 1080, att.Height)
	assert.Equal(t, 0, att.Animated)
	assert.Equal(t, 1, att.Downloaded)
	assert.Equal(t, 2, att.Version)
	assert.NotEmpty(t, att.CreatedAt)
	assert.NotEmpty(t, att.BearRaw)
}

func TestMapBearAttachment_FileType(t *testing.T) {
	row := BearAttachmentRow{
		ZPK:               1,
		ZENT:              8, // file
		ZUNIQUEIDENTIFIER: ptr("FILE-UUID"),
		ZFILENAME:         ptr("document.pdf"),
	}

	att, err := MapBearAttachment(&row)
	require.NoError(t, err)
	assert.Equal(t, "file", att.Type)
}

func TestMapBearAttachment_VideoType(t *testing.T) {
	row := BearAttachmentRow{
		ZPK:               1,
		ZENT:              10, // video
		ZUNIQUEIDENTIFIER: ptr("VIDEO-UUID"),
		ZFILENAME:         ptr("clip.mp4"),
		ZDURATION:         ptr(int64(120)),
	}

	att, err := MapBearAttachment(&row)
	require.NoError(t, err)
	assert.Equal(t, "video", att.Type)
	assert.Equal(t, 120, att.Duration)
}

func TestMapBearBacklink(t *testing.T) {
	row := BearBacklinkRow{
		ZPK:               1,
		ZUNIQUEIDENTIFIER: ptr("BL-UUID-101"),
		ZLINKEDBY:         ptr("NOTE-UUID-A"),
		ZLINKINGTO:        ptr("NOTE-UUID-B"),
		ZTITLE:            ptr("Reference to Note B"),
		ZLOCATION:         ptr(int64(42)),
		ZVERSION:          ptr(int64(1)),
		ZMODIFICATIONDATE: ptr(726842600.0),
	}

	bl, err := MapBearBacklink(&row)
	require.NoError(t, err)

	assert.Len(t, bl.ID, 32)
	assert.Equal(t, "BL-UUID-101", *bl.BearID)
	assert.Equal(t, "NOTE-UUID-A", bl.LinkedByID)
	assert.Equal(t, "NOTE-UUID-B", bl.LinkingToID)
	assert.Equal(t, "Reference to Note B", bl.Title)
	assert.Equal(t, 42, bl.Location)
	assert.Equal(t, 1, bl.Version)
	assert.NotEmpty(t, bl.ModifiedAt)
	assert.NotEmpty(t, bl.BearRaw)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(bl.BearRaw), &raw))
	assert.Equal(t, "BL-UUID-101", raw["ZUNIQUEIDENTIFIER"])
}

func TestMapBearBacklink_NullFields(t *testing.T) {
	row := BearBacklinkRow{ZPK: 1}

	bl, err := MapBearBacklink(&row)
	require.NoError(t, err)

	assert.Nil(t, bl.BearID)
	assert.Empty(t, bl.LinkedByID)
	assert.Empty(t, bl.LinkingToID)
	assert.Empty(t, bl.Title)
	assert.Equal(t, 0, bl.Location)
	assert.Equal(t, 0, bl.Version)
	assert.Empty(t, bl.ModifiedAt)
}

func TestBearRawContainsAllOriginalFields(t *testing.T) {
	row := BearNoteRow{
		ZPK:               42,
		ZUNIQUEIDENTIFIER: ptr("UUID-1"),
		ZTITLE:            ptr("My Title"),
		ZTEXT:             ptr("My Body"),
		ZVERSION:          ptr(int64(7)),
		ZCREATIONDATE:     ptr(726842600.0),
	}

	note, err := MapBearNote(&row)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(note.BearRaw), &raw))

	assert.Equal(t, float64(42), raw["ZPK"])
	assert.Equal(t, "UUID-1", raw["ZUNIQUEIDENTIFIER"])
	assert.Equal(t, "My Title", raw["ZTITLE"])
	assert.Equal(t, "My Body", raw["ZTEXT"])
	assert.Equal(t, float64(7), raw["ZVERSION"])
	assert.Equal(t, 726842600.0, raw["ZCREATIONDATE"])
}
