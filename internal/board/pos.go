package board

import (
	"bytes"
	"encoding/json"
	"os"
)

// ReadPos returns the JSON stored in a position file, compacted, or an empty object when it is missing or not JSON.
// The page and the cloud upload both answer a board together with these positions.
func ReadPos(file string) json.RawMessage {
	data, err := os.ReadFile(file)
	if err != nil || !json.Valid(data) {
		return json.RawMessage("{}")
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return json.RawMessage("{}")
	}

	return buf.Bytes()
}
