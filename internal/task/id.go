package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID produces a stable, sortable-by-prefix opaque identifier. The random
// suffix prevents identifiers from being inferred from titles or paths.
func NewID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("generate identifier: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
