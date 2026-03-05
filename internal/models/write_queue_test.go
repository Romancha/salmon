package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/salmon/internal/models"
)

func TestWriteQueueItem_ConsumerID_JSON(t *testing.T) {
	item := models.WriteQueueItem{
		ID:             1,
		IdempotencyKey: "key-1",
		Action:         "create",
		NoteID:         "n1",
		Payload:        `{"title":"Test"}`,
		Status:         "pending",
		ConsumerID:     "testapp",
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var decoded models.WriteQueueItem
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "testapp", decoded.ConsumerID)
}

func TestWriteQueueItem_ConsumerID_EmptyOmitted(t *testing.T) {
	item := models.WriteQueueItem{
		ID:     1,
		Status: "pending",
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	assert.NotContains(t, string(data), "consumer_id")
}
