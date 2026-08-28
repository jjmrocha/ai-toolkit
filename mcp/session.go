package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jjmrocha/ai-toolkit/helper"
)

const (
	jsonrpcVersion = "2.0"
	methodNotFound = -32601
	answerTimeout  = 5 * time.Second
	requestTimeout = 30 * time.Second
)

type transport interface {
	Write(ctx context.Context, msg string) error
	Output() <-chan string
	Running() bool
	Close()
}

type session struct {
	transport transport
	requestID *seqNum
	requests  *pendingRequest
	handshake map[string]any
}

func newSession(ctx context.Context, command string, args []string, onDisconnect func()) (*session, error) {
	t, err := helper.NewProcess(helper.ProcessConfig{
		Path:       command,
		Args:       args,
		AllowInput: true,
		OnExit: func(error) {
			if onDisconnect != nil {
				onDisconnect()
			}
		},
	})
	if err != nil {
		return nil, err
	}

	s := &session{
		transport: t,
		requestID: newSeqNum(),
		requests:  newPendingRequest(),
	}

	go s.messageProcessor()

	if err := s.initialize(ctx); err != nil {
		s.close()
		return nil, err
	}

	return s, nil
}

func (s *session) messageProcessor() {
	for line := range s.transport.Output() {
		if line == "" {
			continue
		}

		for _, message := range decodeMessages(line) {
			s.handleMessage(message)
		}
	}

	s.requests.failAll(ErrMCPConnectionClosed)
}

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
	if message.Method != "" {
		s.answer(message.Method, message.ID)

		return
	}

	s.settle(message)
}

func (s *session) settle(message serverMessage) {
	id, isResponse := responseID(message.ID)
	if !isResponse {
		return
	}

	if message.Error != nil {
		s.requests.reject(id, fmt.Errorf("MCP server error: %w", message.Error))
		return
	}

	result := message.Result
	if result == nil {
		result = map[string]any{}
	}

	s.requests.resolve(id, result)
}

func responseID(raw any) (int, bool) {
	switch id := raw.(type) {
	case float64:
		return int(id), true
	case string:
		parsed, err := strconv.Atoi(id)

		return parsed, err == nil
	default:
		return 0, false
	}
}

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

	s.handshake = result

	return s.notify(ctx, "notifications/initialized", nil)
}

func (s *session) Request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	id := s.requestID.next()
	pending := s.requests.newRequest(id)

	if err := s.send(ctx, newRequest(id, method, params)); err != nil {
		s.requests.reject(id, err)

		return nil, fmt.Errorf("sending request to MCP server: %w", err)
	}

	result, err := pending.AwaitWithContext(ctx)
	if err == nil {
		return result, nil
	}

	if ctx.Err() == nil {
		return nil, err
	}

	s.requests.reject(id, err)

	if canCancel(method) {
		s.cancelRequest(ctx, id, err)
	}

	return nil, err
}

func (s *session) cancelRequest(ctx context.Context, id int, cause error) {
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

func (s *session) close() {
	s.transport.Close()
}

func (s *session) supportsTools() bool {
	capabilities, ok := s.handshake["capabilities"].(map[string]any)
	if !ok {
		return true
	}

	_, ok = capabilities["tools"]

	return ok
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
