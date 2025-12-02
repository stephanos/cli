package cliext

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"go.temporal.io/api/common/v1"
)

// JSONEncodingMetadata is metadata for JSON-encoded payloads.
var JSONEncodingMetadata = map[string][][]byte{"encoding": {[]byte("json/plain")}}

// Decoder is a function that decodes payload data before creating the payload.
// If decoding fails, it should return an error.
type Decoder func([]byte) ([]byte, error)

// Base64Decoder is a decoder that decodes base64-encoded data using standard encoding.
var Base64Decoder Decoder = func(data []byte) ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(data))
}

// CreatePayloads creates API Payload objects from given data and metadata slices.
// If metadata has an entry at a data index, it is used, otherwise it uses the metadata entry at index 0.
// If decoder is non-nil, it is applied to each data element before creating the payload.
//
// Example:
//
//	payloads, err := cliext.CreatePayloads(
//	    [][]byte{[]byte(`{"key": "value"}`)},
//	    map[string][][]byte{"encoding": {[]byte("json/plain")}},
//	    nil,
//	)
//
//	// With base64 decoding:
//	payloads, err := cliext.CreatePayloads(
//	    [][]byte{[]byte("aGVsbG8gd29ybGQ=")},
//	    map[string][][]byte{"encoding": {[]byte("binary/plain")}},
//	    cliext.Base64Decoder,
//	)
func CreatePayloads(data [][]byte, metadata map[string][][]byte, decoder Decoder) (*common.Payloads, error) {
	ret := &common.Payloads{Payloads: make([]*common.Payload, len(data))}
	for i, in := range data {
		var metadataForIndex = make(map[string][]byte, len(metadata))
		for k, vals := range metadata {
			if len(vals) == 0 {
				continue
			}
			v := vals[0]
			if len(vals) > i {
				v = vals[i]
			}
			// If it's JSON, validate it
			if k == "encoding" && strings.HasPrefix(string(v), "json/") && !json.Valid(in) {
				return nil, fmt.Errorf("input #%v is not valid JSON", i+1)
			}
			metadataForIndex[k] = v
		}
		// Apply decoder if provided
		if decoder != nil {
			var err error
			if in, err = decoder(in); err != nil {
				return nil, fmt.Errorf("input #%v: %w", i+1, err)
			}
		}
		ret.Payloads[i] = &common.Payload{Data: in, Metadata: metadataForIndex}
	}
	return ret, nil
}
