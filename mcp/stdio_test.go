package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMemStdio wires a client to a scripted server: the Nth message the client
// sends is answered with the Nth response, and the stream ends once the script
// runs out, the way a server closing its stdout would end it.
func newMemStdio(responses ...string) (*stdio, *fakeTransport) {
	ft := newScriptedTransport(responses...)

	return newTestStdio(ft), ft
}

// newTestStdio wires a client onto t and starts dispatching, the way newStdIO
// does once it has built its cmdIO.
func newTestStdio(t transport) *stdio {
	s := &stdio{
		transport: t,
		messageID: newSeqNum(),
		requests:  newPendingRequest(),
	}

	go s.dispatch()

	return s
}

func sentMessages(t testing.TB, f *fakeTransport) []map[string]any {
	t.Helper()

	var messages []map[string]any

	for {
		select {
		case line := <-f.written:
			var message map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &message))
			messages = append(messages, message)
		default:
			return messages
		}
	}
}

// fakeTransport is a scripted transport: the test drives what the server says
// with send, and inspects what the client wrote with nextSent.
type fakeTransport struct {
	written chan string
	lines   chan string
	closed  atomic.Bool

	mu       sync.Mutex // guards script
	script   []string
	scripted bool
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		written: make(chan string, 16),
		lines:   make(chan string, 16),
	}
}

func newScriptedTransport(responses ...string) *fakeTransport {
	f := newFakeTransport()
	f.scripted = true

	for _, response := range responses {
		f.script = append(f.script, strings.TrimSuffix(response, "\n"))
	}

	return f
}

func (f *fakeTransport) Write(_ context.Context, msg string) error {
	if f.closed.Load() {
		return ErrProcessClosed
	}

	f.written <- msg

	if f.scripted {
		f.replyNext()
	}

	return nil
}

// replyNext releases the next scripted response, or ends the stream once the
// script is exhausted.
func (f *fakeTransport) replyNext() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.script) == 0 {
		f.Close()

		return
	}

	line := f.script[0]
	f.script = f.script[1:]
	f.lines <- line
}

func (f *fakeTransport) Reader() <-chan string { return f.lines }

func (f *fakeTransport) Running() bool { return !f.closed.Load() }

func (f *fakeTransport) Close() {
	if f.closed.CompareAndSwap(false, true) {
		close(f.lines)
	}
}

func (f *fakeTransport) send(line string) { f.lines <- line }

func (f *fakeTransport) nextSent(t testing.TB) map[string]any {
	t.Helper()

	select {
	case line := <-f.written:
		var message map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &message))

		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the client to write a message")

		return nil
	}
}

func TestStdioDispatch(t *testing.T) {
	t.Run("resolves in-flight requests regardless of response order", func(t *testing.T) {
		// given: two requests in flight, the second one issued after the first is on the wire
		ft := newFakeTransport()
		s := newTestStdio(ft)

		type reply struct {
			result map[string]any
			err    error
		}

		first := make(chan reply, 1)
		second := make(chan reply, 1)

		go func() {
			result, err := s.Request(t.Context(), "first", nil)
			r := reply{result: result, err: err}
			first <- r
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])

		go func() {
			result, err := s.Request(t.Context(), "second", nil)
			r := reply{result: result, err: err}
			second <- r
		}()
		require.Equal(t, float64(2), ft.nextSent(t)["id"])
		// when: the server answers the newest request first
		ft.send(`{"jsonrpc":"2.0","id":2,"result":{"which":"second"}}`)
		ft.send(`{"jsonrpc":"2.0","id":1,"result":{"which":"first"}}`)
		// then: each caller gets its own response
		r2 := <-second
		require.NoError(t, r2.err)
		assert.Equal(t, map[string]any{"which": "second"}, r2.result)

		r1 := <-first
		require.NoError(t, r1.err)
		assert.Equal(t, map[string]any{"which": "first"}, r1.result)
	})

	t.Run("resolves every response in a batched reply", func(t *testing.T) {
		// given: revisions before 2025-06-18 let a server answer with a JSON-RPC batch
		ft := newFakeTransport()
		s := newTestStdio(ft)

		type reply struct {
			result map[string]any
			err    error
		}

		first := make(chan reply, 1)
		second := make(chan reply, 1)

		go func() {
			result, err := s.Request(t.Context(), "first", nil)
			r := reply{result: result, err: err}
			first <- r
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])

		go func() {
			result, err := s.Request(t.Context(), "second", nil)
			r := reply{result: result, err: err}
			second <- r
		}()
		require.Equal(t, float64(2), ft.nextSent(t)["id"])
		// when: both answers arrive on one line
		ft.send(`[{"jsonrpc":"2.0","id":1,"result":{"which":"first"}},{"jsonrpc":"2.0","id":2,"result":{"which":"second"}}]`)
		// then
		r1 := <-first
		require.NoError(t, r1.err)
		assert.Equal(t, map[string]any{"which": "first"}, r1.result)

		r2 := <-second
		require.NoError(t, r2.err)
		assert.Equal(t, map[string]any{"which": "second"}, r2.result)
	})

	t.Run("ignores noise and messages matching no in-flight request", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)

		var result map[string]any
		done := make(chan error, 1)

		go func() {
			var err error
			result, err = s.Request(t.Context(), "tools/list", nil)
			done <- err
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])
		// when: noise, a notification and a stale id arrive before the real response
		ft.send("not-json")
		ft.send(`{"jsonrpc":"2.0","method":"notifications/log","params":{}}`)
		ft.send(`{"jsonrpc":"2.0","id":99,"result":{"ignored":true}}`)
		ft.send(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)
		// then
		require.NoError(t, <-done)
		assert.Equal(t, map[string]any{"ok": true}, result)
	})

	t.Run("ignores a server request reusing an in-flight id", func(t *testing.T) {
		// given: ids are per-direction, so the server may issue its own id 1
		ft := newFakeTransport()
		s := newTestStdio(ft)

		var result map[string]any
		done := make(chan error, 1)

		go func() {
			var err error
			result, err = s.Request(t.Context(), "tools/call", nil)
			done <- err
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])
		// when: the server asks something of its own before answering
		ft.send(`{"jsonrpc":"2.0","id":1,"method":"sampling/createMessage","params":{}}`)
		ft.send(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)
		// then: the caller still gets the real response
		require.NoError(t, <-done)
		assert.Equal(t, map[string]any{"ok": true}, result)
	})

	t.Run("answers a ping with an empty result", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		newTestStdio(ft)
		// when
		ft.send(`{"jsonrpc":"2.0","id":7,"method":"ping"}`)
		// then
		sent := ft.nextSent(t)
		assert.Equal(t, float64(7), sent["id"])
		assert.Equal(t, map[string]any{}, sent["result"])
		assert.NotContains(t, sent, "error")
	})

	t.Run("refuses a server request it cannot serve", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		newTestStdio(ft)
		// when: the server asks for something this client does not implement
		ft.send(`{"jsonrpc":"2.0","id":"abc","method":"sampling/createMessage","params":{}}`)
		// then: the id is echoed verbatim, so a string id survives
		sent := ft.nextSent(t)
		assert.Equal(t, "abc", sent["id"])
		rpcErr, ok := sent["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(-32601), rpcErr["code"])
	})

	t.Run("does not answer a notification", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)
		// when
		ft.send(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`)
		// then: the next request is the first thing written, so nothing answered it
		done := make(chan error, 1)

		go func() {
			_, err := s.Request(t.Context(), "tools/list", nil)
			done <- err
		}()
		assert.Equal(t, "tools/list", ft.nextSent(t)["method"])

		ft.Close()
		<-done
	})

	t.Run("fails an in-flight request when the stream ends", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)

		done := make(chan error, 1)

		go func() {
			_, err := s.Request(t.Context(), "tools/list", nil)
			done <- err
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])
		// when: the server goes away without answering
		ft.Close()
		// then
		assert.ErrorIs(t, <-done, ErrMCPConnectionClosed)
	})

	// The disconnect callback now fires from cmdIO's process-exit path rather than
	// from the end of the stream; TestCmdIONotification covers it.
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
		assert.Empty(t, sentMessages(t, in))
	})

	t.Run("tells the server to stop working on an abandoned request", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)
		ctx, cancel := context.WithCancel(t.Context())

		done := make(chan error, 1)

		go func() {
			_, err := s.Request(ctx, "tools/call", nil)
			done <- err
		}()
		require.Equal(t, float64(1), ft.nextSent(t)["id"])
		// when: the caller gives up before the server answers
		cancel()
		// then
		assert.ErrorIs(t, <-done, context.Canceled)
		sent := ft.nextSent(t)
		assert.Equal(t, "notifications/cancelled", sent["method"])
		expected := map[string]any{"requestId": float64(1), "reason": "context canceled"}
		assert.Equal(t, expected, sent["params"])
	})

	t.Run("does not cancel a request the server already failed", func(t *testing.T) {
		// given
		s, ft := newMemStdio(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}` + "\n")
		// when
		_, err := s.Request(t.Context(), "tools/call", nil)
		// then: the request is over, so there is nothing to cancel
		require.Error(t, err)
		sent := sentMessages(t, ft)
		require.Len(t, sent, 1)
		assert.Equal(t, "tools/call", sent[0]["method"])
	})

	t.Run("does not cancel an abandoned initialize request", func(t *testing.T) {
		// given: the spec forbids cancelling initialize
		ft := newFakeTransport()
		s := newTestStdio(ft)
		ctx, cancel := context.WithCancel(t.Context())

		done := make(chan error, 1)

		go func() {
			_, err := s.Request(ctx, "initialize", nil)
			done <- err
		}()
		require.Equal(t, "initialize", ft.nextSent(t)["method"])
		// when
		cancel()
		// then
		assert.ErrorIs(t, <-done, context.Canceled)
		assert.Empty(t, sentMessages(t, ft))
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

	t.Run("accepts a supported version the server downgrades to", func(t *testing.T) {
		// given: the server does not speak the offered version and answers with an older one
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05"}}` + "\n")
		// when
		err := s.initialize(t.Context())
		// then
		require.NoError(t, err)
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		assert.Equal(t, "notifications/initialized", sent[1]["method"])
	})

	t.Run("identifies the client with a name, title and version", func(t *testing.T) {
		// given
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"` + protocolVersion + `"}}` + "\n")
		// when
		require.NoError(t, s.initialize(t.Context()))
		// then
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		params, ok := sent[0]["params"].(map[string]any)
		require.True(t, ok)
		expected := map[string]any{"name": clientName, "title": clientTitle, "version": clientVersion}
		assert.Equal(t, expected, params["clientInfo"])
	})

	t.Run("rejects a server offering a different protocol version", func(t *testing.T) {
		// given
		s, in := newMemStdio(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1999-01-01"}}` + "\n")
		// when
		err := s.initialize(t.Context())
		// then
		assert.ErrorIs(t, err, ErrUnsupportedProtocolVersion)
		sent := sentMessages(t, in)
		require.Len(t, sent, 1)
		assert.Equal(t, "initialize", sent[0]["method"])
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

// Process lifecycle is cmdIO's job and is covered by TestCmdIOClose,
// TestCmdIORunning and TestCmdIONotification. What is left here is the
// delegation.
func TestStdioConnected(t *testing.T) {
	t.Run("reports the transport as running", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)
		// then
		assert.True(t, s.connected())
	})

	t.Run("reports the transport as stopped once it is closed", func(t *testing.T) {
		// given
		ft := newFakeTransport()
		s := newTestStdio(ft)
		// when
		require.NoError(t, s.close())
		// then
		assert.False(t, s.connected())
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
