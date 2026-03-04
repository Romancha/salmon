package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// BridgeState holds the persistent state of the bridge between sync cycles.
type BridgeState struct {
	LastSyncAt              float64  `json:"last_sync_at"` // Core Data epoch
	KnownNoteIDs            []string `json:"known_note_ids"`
	KnownTagIDs             []string `json:"known_tag_ids"`
	KnownAttachmentIDs      []string `json:"known_attachment_ids"`
	KnownBacklinkIDs        []string `json:"known_backlink_ids"`
	KnownNoteTagPairs       []IDPair `json:"known_note_tag_pairs"`
	KnownPinnedNoteTagPairs []IDPair `json:"known_pinned_note_tag_pairs"`
	JunctionFullScanCounter int      `json:"junction_full_scan_counter"`
}

// IDPair represents a note-tag UUID pair for junction table state tracking.
type IDPair struct {
	NoteUUID string `json:"note_uuid"`
	TagUUID  string `json:"tag_uuid"`
}

// loadState reads the bridge state from disk. Returns nil state and no error if file doesn't exist.
func loadState(path string) (*BridgeState, error) {
	data, err := os.ReadFile(path) //nolint:gosec // state file path from trusted config
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil // nil state means initial sync needed
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state BridgeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// saveState writes the bridge state to disk atomically (write to temp, then rename).
func saveState(path string, state *BridgeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename state file: %w", err)
	}

	return nil
}
