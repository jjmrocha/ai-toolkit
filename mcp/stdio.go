package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	jsonrpcVersion = "2.0"
	methodNotFound = -32601
	// answerTimeout bounds a reply to a server-initiated request. It runs on the
	// dispatch goroutine, so an unbounded write would stall every inbound response.
	answerTimeout = 5 * time.Second
)

type transport interface {
	Write(ctx context.Context, msg string) error
	Reader() <-chan string
	Running() bool
	Close()
}

type stdio struct {
	transport transport
	messageID *seqNum
	requests  *pendingRequest
}

func newStdIO(ctx context.Context, command string, args []string, fn func()) (*stdio, error) {
	t, err := newCmdIO(command, args, func(error) {
		if fn != nil {
			fn()
		}
	})
	if err != nil {
		return nil, err
	}

	s := &stdio{
		transport: t,
		messageID: newSeqNum(),
		requests:  newPendingRequest(),
	}

	go s.dispatch()

	if err := s.initialize(ctx); err != nil {
		_ = s.close()
		return nil, err
	}

	return s, nil
}

func (s *stdio) dispatch() {
	for line := range s.transport.Reader() {
		for _, message := range decodeMessages(line) {
			s.handleMessage(message)
		}
	}

	s.requests.failAll(ErrMCPConnectionClosed)
}

// decodeMessages handles both shapes a line may carry: revisions older than
// 2025-06-18 let a server answer with a JSON-RPC batch.
func decodeMessages(line string) []map[string]any {
	raw := []byte(line)

	var message map[string]any
	if json.Unmarshal(raw, &message) == nil {
		return []map[string]any{message}
	}

	var batch []map[string]any
	if json.Unmarshal(raw, &batch) == nil {
		return batch
	}

	return nil
}

func (s *stdio) handleMessage(message map[string]any) {
	// Ids are per-direction, so a server request may reuse one we have in flight.
	if method, isRequest := message["method"].(string); isRequest {
		s.answer(method, message["id"])

		return
	}

	id, isResponse := message["id"].(float64)
	if !isResponse {
		return
	}

	if rpcErr, failed := message["error"]; failed {
		s.requests.reject(int(id), fmt.Errorf("MCP server error: %s", errorMessage(rpcErr)))
		return
	}

	result, _ := message["result"].(map[string]any)
	s.requests.resolve(int(id), orEmpty(result))
}

// answer replies to a server-initiated request. Notifications carry no id and
// must not be answered.
func (s *stdio) answer(method string, id any) {
	if id == nil {
		return
	}

	response := map[string]any{
		"jsonrpc": jsonrpcVersion,
		"id":      id,
	}

	if canServe(method) {
		response["result"] = map[string]any{}
	} else {
		response["error"] = map[string]any{
			"code":    methodNotFound,
			"message": "method not supported by this client",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
	defer cancel()

	_ = s.sendMessage(ctx, response)
}

func (s *stdio) initialize(ctx context.Context) error {
	result, err := s.Request(ctx, "initialize", initializeParams())
	if err != nil {
		return err
	}

	if err := acceptProtocolVersion(result); err != nil {
		return err
	}

	return s.notify(ctx, "notifications/initialized", nil)
}

func (s *stdio) Request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	id := s.messageID.next()
	f := s.requests.add(id)

	message := map[string]any{
		"jsonrpc": jsonrpcVersion,
		"id":      id,
		"method":  method,
		"params":  orEmpty(params),
	}

	if err := s.sendMessage(ctx, message); err != nil {
		s.requests.reject(id, err)

		return nil, fmt.Errorf("sending request to MCP server: %w", err)
	}

	// reject is a no-op once the server has answered, so it only drops the entry
	// an abandoned request would otherwise leave behind.
	result, err := f.AwaitWithContext(ctx)
	if err != nil {
		s.requests.reject(id, err)
		s.cancelRequest(ctx, method, id, err)

		return nil, err
	}

	return result, nil
}

// cancelRequest tells the server to stop working on a request the caller has
// abandoned. Only the caller giving up is worth cancelling: a server that
// already answered has nothing left to stop.
func (s *stdio) cancelRequest(ctx context.Context, method string, id int, cause error) {
	if ctx.Err() == nil || !canCancel(method) {
		return
	}

	// ctx is the one that just expired, so send on a detached copy.
	_ = s.notify(context.WithoutCancel(ctx), "notifications/cancelled", map[string]any{
		"requestId": id,
		"reason":    cause.Error(),
	})
}

func (s *stdio) notify(ctx context.Context, method string, params map[string]any) error {
	notification := map[string]any{
		"jsonrpc": jsonrpcVersion,
		"method":  method,
		"params":  orEmpty(params),
	}

	if err := s.sendMessage(ctx, notification); err != nil {
		return fmt.Errorf("sending notification to MCP server: %w", err)
	}

	return nil
}

func (s *stdio) close() error {
	s.transport.Close()
	return nil
}

func (s *stdio) connected() bool {
	return s.transport.Running()
}

func (s *stdio) sendMessage(ctx context.Context, message map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshaling message to JSON: %w", err)
	}

	return s.transport.Write(ctx, string(data))
}

func orEmpty(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

func errorMessage(rpcErr any) string {
	if obj, ok := rpcErr.(map[string]any); ok {
		if msg, ok := obj["message"].(string); ok {
			return msg
		}
	}

	return fmt.Sprintf("%v", rpcErr)
}
