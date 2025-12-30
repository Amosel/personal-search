package model

import (
	"crypto/sha256"
	"encoding/hex"
)

// StableID generates a deterministic document ID from conversation and message IDs.
// Panics if any input is empty (programmer error).
func StableID(conversationID, messageID string) string {
	if conversationID == "" || messageID == "" {
		panic("StableID: empty input not allowed")
	}
	h := sha256.Sum256([]byte(conversationID + "|" + messageID))
	return hex.EncodeToString(h[:])
}
