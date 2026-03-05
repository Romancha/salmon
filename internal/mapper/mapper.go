package mapper

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/romancha/salmon/internal/models"
)

// CoreDataEpochOffset is the difference in seconds between Unix epoch (1970-01-01)
// and Core Data epoch (2001-01-01).
const CoreDataEpochOffset = 978307200

// BearNoteRow represents a raw row from Bear's ZSFNOTE table.
type BearNoteRow struct {
	ZPK                           int64
	ZUNIQUEIDENTIFIER             *string
	ZTITLE                        *string
	ZSUBTITLE                     *string
	ZTEXT                         *string
	ZARCHIVED                     *int64
	ZENCRYPTED                    *int64
	ZHASFILES                     *int64
	ZHASIMAGES                    *int64
	ZHASSOURCECODE                *int64
	ZLOCKED                       *int64
	ZPINNED                       *int64
	ZSHOWNINTODAYWIDGET           *int64
	ZTRASHED                      *int64
	ZPERMANENTLYDELETED           *int64
	ZSKIPSYNC                     *int64
	ZTODOCOMPLETED                *int64
	ZTODOINCOMPLETED              *int64
	ZVERSION                      *int64
	ZCREATIONDATE                 *float64
	ZMODIFICATIONDATE             *float64
	ZARCHIVEDDATE                 *float64
	ZENCRYPTIONDATE               *float64
	ZLOCKEDDATE                   *float64
	ZPINNEDDATE                   *float64
	ZTRASHEDDATE                  *float64
	ZORDERDATE                    *float64
	ZCONFLICTUNIQUEIDENTIFIERDATE *float64
	ZLASTEDITINGDEVICE            *string
	ZCONFLICTUNIQUEIDENTIFIER     *string
	ZENCRYPTIONUNIQUEIDENTIFIER   *string
	ZENCRYPTEDDATA                []byte
}

// BearTagRow represents a raw row from Bear's ZSFNOTETAG table.
type BearTagRow struct {
	ZPK                   int64
	ZUNIQUEIDENTIFIER     *string
	ZTITLE                *string
	ZPINNED               *int64
	ZISROOT               *int64
	ZHIDESUBTAGSNOTES     *int64
	ZSORTING              *int64
	ZSORTINGDIRECTION     *int64
	ZENCRYPTED            *int64
	ZVERSION              *int64
	ZMODIFICATIONDATE     *float64
	ZPINNEDDATE           *float64
	ZPINNEDNOTESDATE      *float64
	ZENCRYPTEDDATE        *float64
	ZHIDESUBTAGSNOTESDATE *float64
	ZSORTINGDATE          *float64
	ZSORTINGDIRECTIONDATE *float64
	ZTAGCONDATE           *float64
	ZTAGCON               *string
}

// BearAttachmentRow represents a raw row from Bear's ZSFNOTEFILE table.
type BearAttachmentRow struct {
	ZPK                         int64
	ZENT                        int64 // Z_ENT: 8=file, 9=image, 10=video
	ZUNIQUEIDENTIFIER           *string
	ZNOTE                       *string // resolved UUID of parent note (via JOIN)
	ZFILENAME                   *string
	ZNORMALIZEDFILEEXTENSION    *string
	ZFILESIZE                   *int64
	ZINDEX                      *int64
	ZWIDTH                      *int64
	ZHEIGHT                     *int64
	ZANIMATED                   *int64
	ZDURATION                   *int64
	ZWIDTH1                     *int64
	ZHEIGHT1                    *int64
	ZDOWNLOADED                 *int64
	ZENCRYPTED                  *int64
	ZPERMANENTLYDELETED         *int64
	ZSKIPSYNC                   *int64
	ZUNUSED                     *int64
	ZUPLOADED                   *int64
	ZVERSION                    *int64
	ZCREATIONDATE               *float64
	ZMODIFICATIONDATE           *float64
	ZINSERTIONDATE              *float64
	ZENCRYPTIONDATE             *float64
	ZUNUSEDDATE                 *float64
	ZUPLOADEDDATE               *float64
	ZSEARCHTEXTDATE             *float64
	ZLASTEDITINGDEVICE          *string
	ZENCRYPTIONUNIQUEIDENTIFIER *string
	ZSEARCHTEXT                 *string
	ZENCRYPTEDDATA              []byte
}

// BearBacklinkRow represents a raw row from Bear's ZSFNOTEBACKLINK table.
type BearBacklinkRow struct {
	ZPK               int64
	ZUNIQUEIDENTIFIER *string
	ZLINKEDBY         *string // resolved UUID of the linking note (via JOIN)
	ZLINKINGTO        *string // resolved UUID of the linked note (via JOIN)
	ZTITLE            *string
	ZLOCATION         *int64
	ZVERSION          *int64
	ZMODIFICATIONDATE *float64
}

// GenerateID produces a hub UUID as hex(randomblob(16)).
func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate hub id: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// ConvertCoreDataDate converts a Core Data epoch timestamp to ISO 8601 (RFC3339).
// Returns empty string for nil input.
func ConvertCoreDataDate(ts *float64) string {
	if ts == nil {
		return ""
	}

	unixSec := int64(*ts) + CoreDataEpochOffset
	nsec := int64((*ts - float64(int64(*ts))) * 1e9)

	return time.Unix(unixSec, nsec).UTC().Format(time.RFC3339)
}

// ConvertAttachmentType maps Bear Z_ENT value to attachment type string.
func ConvertAttachmentType(zent int64) string {
	switch zent {
	case 8:
		return "file"
	case 9:
		return "image"
	case 10:
		return "video"
	default:
		return "file"
	}
}

// MapBearNote converts a BearNoteRow to a models.Note with a generated hub UUID.
func MapBearNote(row *BearNoteRow) (models.Note, error) {
	id, err := GenerateID()
	if err != nil {
		return models.Note{}, fmt.Errorf("map bear note: %w", err)
	}

	bearRaw, err := json.Marshal(row)
	if err != nil {
		return models.Note{}, fmt.Errorf("marshal bear_raw for note: %w", err)
	}

	return models.Note{
		ID:                 id,
		BearID:             row.ZUNIQUEIDENTIFIER,
		Title:              derefStr(row.ZTITLE),
		Subtitle:           derefStr(row.ZSUBTITLE),
		Body:               derefStr(row.ZTEXT),
		Archived:           derefInt(row.ZARCHIVED),
		Encrypted:          derefInt(row.ZENCRYPTED),
		HasFiles:           derefInt(row.ZHASFILES),
		HasImages:          derefInt(row.ZHASIMAGES),
		HasSourceCode:      derefInt(row.ZHASSOURCECODE),
		Locked:             derefInt(row.ZLOCKED),
		Pinned:             derefInt(row.ZPINNED),
		ShownInToday:       derefInt(row.ZSHOWNINTODAYWIDGET),
		Trashed:            derefInt(row.ZTRASHED),
		PermanentlyDeleted: derefInt(row.ZPERMANENTLYDELETED),
		SkipSync:           derefInt(row.ZSKIPSYNC),
		TodoCompleted:      derefInt(row.ZTODOCOMPLETED),
		TodoIncompleted:    derefInt(row.ZTODOINCOMPLETED),
		Version:            derefInt(row.ZVERSION),
		CreatedAt:          ConvertCoreDataDate(row.ZCREATIONDATE),
		ModifiedAt:         ConvertCoreDataDate(row.ZMODIFICATIONDATE),
		ArchivedAt:         ConvertCoreDataDate(row.ZARCHIVEDDATE),
		EncryptedAt:        ConvertCoreDataDate(row.ZENCRYPTIONDATE),
		LockedAt:           ConvertCoreDataDate(row.ZLOCKEDDATE),
		PinnedAt:           ConvertCoreDataDate(row.ZPINNEDDATE),
		TrashedAt:          ConvertCoreDataDate(row.ZTRASHEDDATE),
		OrderDate:          ConvertCoreDataDate(row.ZORDERDATE),
		ConflictIDDate:     ConvertCoreDataDate(row.ZCONFLICTUNIQUEIDENTIFIERDATE),
		LastEditingDevice:  derefStr(row.ZLASTEDITINGDEVICE),
		ConflictID:         derefStr(row.ZCONFLICTUNIQUEIDENTIFIER),
		EncryptionID:       derefStr(row.ZENCRYPTIONUNIQUEIDENTIFIER),
		EncryptedData:      row.ZENCRYPTEDDATA,
		SyncStatus:         "synced",
		BearRaw:            string(bearRaw),
	}, nil
}

// MapBearTag converts a BearTagRow to a models.Tag with a generated hub UUID.
func MapBearTag(row *BearTagRow) (models.Tag, error) {
	id, err := GenerateID()
	if err != nil {
		return models.Tag{}, fmt.Errorf("map bear tag: %w", err)
	}

	bearRaw, err := json.Marshal(row)
	if err != nil {
		return models.Tag{}, fmt.Errorf("marshal bear_raw for tag: %w", err)
	}

	return models.Tag{
		ID:                 id,
		BearID:             row.ZUNIQUEIDENTIFIER,
		Title:              derefStr(row.ZTITLE),
		Pinned:             derefInt(row.ZPINNED),
		IsRoot:             derefInt(row.ZISROOT),
		HideSubtagsNotes:   derefInt(row.ZHIDESUBTAGSNOTES),
		Sorting:            derefInt(row.ZSORTING),
		SortingDirection:   derefInt(row.ZSORTINGDIRECTION),
		Encrypted:          derefInt(row.ZENCRYPTED),
		Version:            derefInt(row.ZVERSION),
		ModifiedAt:         ConvertCoreDataDate(row.ZMODIFICATIONDATE),
		PinnedAt:           ConvertCoreDataDate(row.ZPINNEDDATE),
		PinnedNotesAt:      ConvertCoreDataDate(row.ZPINNEDNOTESDATE),
		EncryptedAt:        ConvertCoreDataDate(row.ZENCRYPTEDDATE),
		HideSubtagsNotesAt: ConvertCoreDataDate(row.ZHIDESUBTAGSNOTESDATE),
		SortingAt:          ConvertCoreDataDate(row.ZSORTINGDATE),
		SortingDirectionAt: ConvertCoreDataDate(row.ZSORTINGDIRECTIONDATE),
		TagConDate:         ConvertCoreDataDate(row.ZTAGCONDATE),
		TagCon:             derefStr(row.ZTAGCON),
		BearRaw:            string(bearRaw),
	}, nil
}

// MapBearAttachment converts a BearAttachmentRow to a models.Attachment with a generated hub UUID.
func MapBearAttachment(row *BearAttachmentRow) (models.Attachment, error) {
	id, err := GenerateID()
	if err != nil {
		return models.Attachment{}, fmt.Errorf("map bear attachment: %w", err)
	}

	bearRaw, err := json.Marshal(row)
	if err != nil {
		return models.Attachment{}, fmt.Errorf("marshal bear_raw for attachment: %w", err)
	}

	return models.Attachment{
		ID:                  id,
		BearID:              row.ZUNIQUEIDENTIFIER,
		NoteID:              derefStr(row.ZNOTE),
		Type:                ConvertAttachmentType(row.ZENT),
		Filename:            derefStr(row.ZFILENAME),
		NormalizedExtension: derefStr(row.ZNORMALIZEDFILEEXTENSION),
		FileSize:            derefInt64(row.ZFILESIZE),
		FileIndex:           derefInt(row.ZINDEX),
		Width:               derefInt(row.ZWIDTH),
		Height:              derefInt(row.ZHEIGHT),
		Animated:            derefInt(row.ZANIMATED),
		Duration:            derefInt(row.ZDURATION),
		Width1:              derefInt(row.ZWIDTH1),
		Height1:             derefInt(row.ZHEIGHT1),
		Downloaded:          derefInt(row.ZDOWNLOADED),
		Encrypted:           derefInt(row.ZENCRYPTED),
		PermanentlyDeleted:  derefInt(row.ZPERMANENTLYDELETED),
		SkipSync:            derefInt(row.ZSKIPSYNC),
		Unused:              derefInt(row.ZUNUSED),
		Uploaded:            derefInt(row.ZUPLOADED),
		Version:             derefInt(row.ZVERSION),
		CreatedAt:           ConvertCoreDataDate(row.ZCREATIONDATE),
		ModifiedAt:          ConvertCoreDataDate(row.ZMODIFICATIONDATE),
		InsertedAt:          ConvertCoreDataDate(row.ZINSERTIONDATE),
		EncryptedAt:         ConvertCoreDataDate(row.ZENCRYPTIONDATE),
		UnusedAt:            ConvertCoreDataDate(row.ZUNUSEDDATE),
		UploadedAt:          ConvertCoreDataDate(row.ZUPLOADEDDATE),
		SearchTextAt:        ConvertCoreDataDate(row.ZSEARCHTEXTDATE),
		LastEditingDevice:   derefStr(row.ZLASTEDITINGDEVICE),
		EncryptionID:        derefStr(row.ZENCRYPTIONUNIQUEIDENTIFIER),
		SearchText:          derefStr(row.ZSEARCHTEXT),
		EncryptedData:       row.ZENCRYPTEDDATA,
		BearRaw:             string(bearRaw),
	}, nil
}

// MapBearBacklink converts a BearBacklinkRow to a models.Backlink with a generated hub UUID.
func MapBearBacklink(row *BearBacklinkRow) (models.Backlink, error) {
	id, err := GenerateID()
	if err != nil {
		return models.Backlink{}, fmt.Errorf("map bear backlink: %w", err)
	}

	bearRaw, err := json.Marshal(row)
	if err != nil {
		return models.Backlink{}, fmt.Errorf("marshal bear_raw for backlink: %w", err)
	}

	return models.Backlink{
		ID:          id,
		BearID:      row.ZUNIQUEIDENTIFIER,
		LinkedByID:  derefStr(row.ZLINKEDBY),
		LinkingToID: derefStr(row.ZLINKINGTO),
		Title:       derefStr(row.ZTITLE),
		Location:    derefInt(row.ZLOCATION),
		Version:     derefInt(row.ZVERSION),
		ModifiedAt:  ConvertCoreDataDate(row.ZMODIFICATIONDATE),
		BearRaw:     string(bearRaw),
	}, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func derefInt(i *int64) int {
	if i == nil {
		return 0
	}

	return int(*i)
}

func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}

	return *i
}
