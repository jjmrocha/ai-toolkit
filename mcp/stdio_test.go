package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemStdio(responses ...string) (*stdio, *bytes.Buffer) {
	in := &bytes.Buffer{}
	s := &stdio{
		in:  in,
		out: strings.NewReader(strings.Join(responses, "")),
	}
	return s, in
}

func sentMessages(t testing.TB, in *bytes.Buffer) []map[string]any {
	t.Helper()

	var messages []map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(in.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		var message map[string]any
		require.NoError(t, json.Unmarshal(line, &message))
		messages = append(messages, message)
	}

	return messages
}

func TestRequest(t *testing.T) {
	t.Run("frames the request and returns the result", func(t *testing.T) {
		// given
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}` + "\n")
		// when
		result, err := s.Request(t.Context(), "tools/list", nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"tools": []any{}}, result)
		sent := sentMessages(t, in)
		require.Len(t, sent, 1)
		assert.Equal(t, "2.0", sent[0]["jsonrpc"])
		assert.Equal(t, float64(1), sent[0]["id"])
		assert.Equal(t, "tools/list", sent[0]["method"])
		assert.Equal(t, map[string]any{}, sent[0]["params"])
	})

	t.Run("returns an error when the server responds with a JSON-RPC error", func(t *testing.T) {
		// given
		s, _ := newMemStdio(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}` + "\n")
		// when
		result, err := s.Request(t.Context(), "tools/list", nil)
		// then
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("does not write when the context is already cancelled", func(t *testing.T) {
		// given
		s, in := newMemStdio("")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		result, err := s.Request(ctx, "tools/list", nil)
		// then
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
		assert.Empty(t, in.Bytes())
	})
}

func TestRead(t *testing.T) {
	t.Run("skips noise and non-matching ids before the matching response", func(t *testing.T) {
		// given
		s, _ := newMemStdio(
			"not-json\n",
			`{"jsonrpc":"2.0","method":"notifications/log","params":{}}`+"\n",
			`{"jsonrpc":"2.0","id":99,"result":{"ignored":true}}`+"\n",
			`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`+"\n",
		)
		// when
		result, err := s.read(t.Context(), 1)
		// then
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"ok": true}, result)
	})

	t.Run("returns ErrMCPConnectionClosed when the stream ends before a match", func(t *testing.T) {
		// given
		s, _ := newMemStdio(`{"jsonrpc":"2.0","id":99,"result":{}}` + "\n")
		// when
		result, err := s.read(t.Context(), 1)
		// then
		assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		assert.Nil(t, result)
	})
}

func TestInitialize(t *testing.T) {
	t.Run("completes the handshake when the protocol version matches", func(t *testing.T) {
		// given
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"` + protocolVersion + `"}}` + "\n")
		// when
		err := s.initialize(t.Context())
		// then
		require.NoError(t, err)
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		assert.Equal(t, "initialize", sent[0]["method"])
		assert.Equal(t, "notifications/initialized", sent[1]["method"])
	})

	t.Run("rejects a server offering a different protocol version", func(t *testing.T) {
		// given
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1999-01-01"}}` + "\n")
		// when
		err := s.initialize(t.Context())
		// then
		assert.ErrorIs(t, err, ErrUnsupportedProtocolVersion)
		assert.NotContains(t, in.String(), "notifications/initialized")
	})

	t.Run("rejects a server that omits the protocol version", func(t *testing.T) {
		// given
		s, _ := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n")
		// when
		err := s.initialize(t.Context())
		// then
		assert.ErrorIs(t, err, ErrUnsupportedProtocolVersion)
	})
}

func TestStdioClose(t *testing.T) {
	t.Run("is a no-op when no process was started", func(t *testing.T) {
		// given
		s := &stdio{}
		// when
		err := s.close()
		// then
		assert.NoError(t, err)
	})
}

func TestOrEmpty(t *testing.T) {
	t.Run("returns an empty map for nil params", func(t *testing.T) {
		// when
		result := orEmpty(nil)
		// then
		assert.Equal(t, map[string]any{}, result)
	})

	t.Run("returns the same map for non-nil params", func(t *testing.T) {
		// given
		params := map[string]any{"a": 1}
		// when
		result := orEmpty(params)
		// then
		assert.Equal(t, params, result)
	})
}

func TestErrorMessage(t *testing.T) {
	t.Run("returns the message field of a JSON-RPC error object", func(t *testing.T) {
		// when
		result := errorMessage(map[string]any{"code": -32000, "message": "boom"})
		// then
		assert.Equal(t, "boom", result)
	})

	t.Run("falls back to a formatted value when there is no message field", func(t *testing.T) {
		// when
		result := errorMessage("plain error")
		// then
		assert.Equal(t, "plain error", result)
	})
}
