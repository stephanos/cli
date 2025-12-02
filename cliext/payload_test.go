package cliext

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePayloads(t *testing.T) {
	t.Run("single JSON payload", func(t *testing.T) {
		data := [][]byte{[]byte(`{"key": "value"}`)}

		payloads, err := CreatePayloads(data, JSONEncodingMetadata, nil)

		require.NoError(t, err)
		require.NotNil(t, payloads)
		require.Len(t, payloads.Payloads, 1)
		assert.Equal(t, data[0], payloads.Payloads[0].Data)
		assert.Equal(t, []byte("json/plain"), payloads.Payloads[0].Metadata["encoding"])
	})

	t.Run("multiple payloads with shared metadata", func(t *testing.T) {
		data := [][]byte{
			[]byte(`{"a": 1}`),
			[]byte(`{"b": 2}`),
			[]byte(`{"c": 3}`),
		}

		payloads, err := CreatePayloads(data, JSONEncodingMetadata, nil)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 3)
		for i, p := range payloads.Payloads {
			assert.Equal(t, data[i], p.Data)
			assert.Equal(t, []byte("json/plain"), p.Metadata["encoding"])
		}
	})

	t.Run("multiple payloads with per-index metadata", func(t *testing.T) {
		data := [][]byte{
			[]byte(`{"a": 1}`),
			[]byte(`plain text`),
		}
		metadata := map[string][][]byte{
			"encoding": {[]byte("json/plain"), []byte("text/plain")},
		}

		payloads, err := CreatePayloads(data, metadata, nil)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 2)
		assert.Equal(t, []byte("json/plain"), payloads.Payloads[0].Metadata["encoding"])
		assert.Equal(t, []byte("text/plain"), payloads.Payloads[1].Metadata["encoding"])
	})

	t.Run("invalid JSON with json encoding", func(t *testing.T) {
		data := [][]byte{[]byte(`{invalid}`)}

		_, err := CreatePayloads(data, JSONEncodingMetadata, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not valid JSON")
	})

	t.Run("base64 encoded data with Base64Decoder", func(t *testing.T) {
		original := []byte(`hello world`)
		encoded := base64.StdEncoding.EncodeToString(original)
		data := [][]byte{[]byte(encoded)}
		metadata := map[string][][]byte{
			"encoding": {[]byte("binary/plain")},
		}

		payloads, err := CreatePayloads(data, metadata, Base64Decoder)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 1)
		assert.Equal(t, original, payloads.Payloads[0].Data)
	})

	t.Run("invalid base64 with Base64Decoder", func(t *testing.T) {
		data := [][]byte{[]byte("not-valid-base64!!!")}
		metadata := map[string][][]byte{}

		_, err := CreatePayloads(data, metadata, Base64Decoder)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input #1")
	})

	t.Run("custom decoder", func(t *testing.T) {
		data := [][]byte{[]byte("HELLO")}
		metadata := map[string][][]byte{}

		// Custom decoder that lowercases the input
		lowercaseDecoder := func(in []byte) ([]byte, error) {
			result := make([]byte, len(in))
			for i, b := range in {
				if b >= 'A' && b <= 'Z' {
					result[i] = b + 32
				} else {
					result[i] = b
				}
			}
			return result, nil
		}

		payloads, err := CreatePayloads(data, metadata, lowercaseDecoder)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 1)
		assert.Equal(t, []byte("hello"), payloads.Payloads[0].Data)
	})

	t.Run("custom decoder error", func(t *testing.T) {
		data := [][]byte{[]byte("data")}
		metadata := map[string][][]byte{}

		failingDecoder := func(in []byte) ([]byte, error) {
			return nil, errors.New("decode failed")
		}

		_, err := CreatePayloads(data, metadata, failingDecoder)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input #1")
		assert.Contains(t, err.Error(), "decode failed")
	})

	t.Run("empty metadata", func(t *testing.T) {
		data := [][]byte{[]byte("raw data")}
		metadata := map[string][][]byte{}

		payloads, err := CreatePayloads(data, metadata, nil)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 1)
		assert.Equal(t, []byte("raw data"), payloads.Payloads[0].Data)
		assert.Empty(t, payloads.Payloads[0].Metadata)
	})

	t.Run("nil metadata values are skipped", func(t *testing.T) {
		data := [][]byte{[]byte("data")}
		metadata := map[string][][]byte{
			"empty": {},
		}

		payloads, err := CreatePayloads(data, metadata, nil)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 1)
		_, exists := payloads.Payloads[0].Metadata["empty"]
		assert.False(t, exists)
	})

	t.Run("binary data without json validation", func(t *testing.T) {
		data := [][]byte{{0x00, 0x01, 0xFF}}
		metadata := map[string][][]byte{
			"encoding": {[]byte("binary/plain")},
		}

		payloads, err := CreatePayloads(data, metadata, nil)

		require.NoError(t, err)
		require.Len(t, payloads.Payloads, 1)
		assert.Equal(t, []byte{0x00, 0x01, 0xFF}, payloads.Payloads[0].Data)
	})

	t.Run("empty data slice", func(t *testing.T) {
		data := [][]byte{}
		metadata := map[string][][]byte{}

		payloads, err := CreatePayloads(data, metadata, nil)

		require.NoError(t, err)
		require.NotNil(t, payloads)
		assert.Empty(t, payloads.Payloads)
	})
}
