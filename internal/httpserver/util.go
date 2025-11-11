package httpserver

import (
	"encoding/json"
	"io"
)

// decodeJSON decodes a JSON request body into the destination struct.
// The reader will be closed after decoding.
func decodeJSON(r io.ReadCloser, dest any) error {
	defer r.Close()
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}
