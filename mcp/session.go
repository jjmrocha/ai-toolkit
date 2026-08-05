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

// request frames an outbound call. A zero ID marks a notification: omitempty
// keeps it off the wire.
type request struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

// response answers a server-initiated request. ID is any because it is echoed
// verbatim and servers may use string ids. Result is any because omitempty on
// a map would drop the empty result an accepted ping must carry.
type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

// serverMessage is anything the server may put on the wire: an answer to one
// of our requests (ID set, Method empty) or a request or notification of its
// own (Method set).
type serverMessage struct {
	ID     any            `json:"id"`
	Method string         `json:"method"`
	Result map[string]any `json:"result"`
	Error  *rpcError      `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

type session struct {
	transport transport
	requestID *seqNum
	requests  *pendingRequest
}

func newSession(ctx context.Context, command string, args []string, onDisconnect func()) (*session, error) {
	t, err := newStdioTransport(command, args, func(error) {
		if onDisconnect != nil {
			onDisconnect()
		}
	})
	if err != nil {
		return nil, err
	}

	s := &session{
		transport: t,
		requestID: newSeqNum(),
		requests:  newPendingRequest(),
	}

	go s.dispatch()

	if err := s.initialize(ctx); err != nil {
		_ = s.close()
		return nil, err
	}

	return s, nil
}

func (s *session) dispatch() {
	for line := range s.transport.Reader() {
		for _, message := range decodeMessages(line) {
			s.handleMessage(message)
		}
	}

	s.requests.failAll(ErrMCPConnectionClosed)
}

// decodeMessages handles both shapes a line may carry: revisions older than
// 2025-06-18 let a server answer with a JSON-RPC batch.
func decodeMessages(line string) []serverMessage {
	raw := []byte(line)

	var message serverMessage
	if json.Unmarshal(raw, &message) == nil {
		return []serverMessage{message}
	}

	var batch []serverMessage
	if json.Unmarshal(raw, &batch) == nil {
		return batch
	}

	return nil
}

func (s *session) handleMessage(message serverMessage) {
	// Ids are per-direction, so a server request may reuse one we have in flight.
	if message.Method != "" {
		s.answer(message.Method, message.ID)

		return
	}

	s.settle(message)
}

// settle completes the pending request a server response belongs to.
func (s *session) settle(message serverMessage) {
	id, isResponse := message.ID.(float64)
	if !isResponse {
		return
	}

	if message.Error != nil {
		s.requests.reject(int(id), fmt.Errorf("MCP server error: %w", message.Error))
		return
	}

	result := message.Result
	if result == nil {
		result = map[string]any{}
	}

	s.requests.resolve(int(id), result)
}

// answer replies to a server-initiated request. Notifications carry no id and
// must not be answered.
func (s *session) answer(method string, id any) {
	if id == nil {
		return
	}

	reply := response{JSONRPC: jsonrpcVersion, ID: id}

	if canServe(method) {
		reply.Result = map[string]any{}
	} else {
		reply.Error = &rpcError{Code: methodNotFound, Message: "method not supported by this client"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
	defer cancel()

	_ = s.send(ctx, reply)
}

func (s *session) initialize(ctx context.Context) error {
	result, err := s.Request(ctx, "initialize", initializeParams())
	if err != nil {
		return err
	}

	if err := acceptProtocolVersion(result); err != nil {
		return err
	}

	return s.notify(ctx, "notifications/initialized", nil)
}

func (s *session) Request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	id := s.requestID.next()
	pending := s.requests.add(id)

	if err := s.send(ctx, newRequest(id, method, params)); err != nil {
		s.requests.reject(id, err)

		return nil, fmt.Errorf("sending request to MCP server: %w", err)
	}

	// reject is a no-op once the server has answered, so it only drops the entry
	// an abandoned request would otherwise leave behind.
	result, err := pending.AwaitWithContext(ctx)
	if err != nil {
		s.requests.reject(id, err)

		abandoned := ctx.Err() != nil
		if abandoned && canCancel(method) {
			s.cancelRequest(ctx, id, err)
		}

		return nil, err
	}

	return result, nil
}

// cancelRequest tells the server to stop working on a request the caller has
// abandoned.
func (s *session) cancelRequest(ctx context.Context, id int, cause error) {
	// ctx is the one that just expired, so send on a detached copy.
	_ = s.notify(context.WithoutCancel(ctx), "notifications/cancelled", map[string]any{
		"requestId": id,
		"reason":    cause.Error(),
	})
}

func (s *session) notify(ctx context.Context, method string, params map[string]any) error {
	if err := s.send(ctx, newRequest(0, method, params)); err != nil {
		return fmt.Errorf("sending notification to MCP server: %w", err)
	}

	return nil
}

func (s *session) close() error {
	s.transport.Close()
	return nil
}

func (s *session) connected() bool {
	return s.transport.Running()
}

func newRequest(id int, method string, params map[string]any) request {
	if params == nil {
		params = map[string]any{}
	}

	return request{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

func (s *session) send(ctx context.Context, message any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshaling message to JSON: %w", err)
	}

	return s.transport.Write(ctx, string(data))
}
