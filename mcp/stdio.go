package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	protocolVersion = "2025-06-18"
	closeTimeout    = 5 * time.Second
	clientName      = "ai-toolkit"
	clientVersion   = "0.1.0"
)

// stdio is a synchronous JSON-RPC transport over an MCP server child process.
// mu serializes Request so at most one request is in flight at a time; close
// intentionally does not take mu, so it can abort a request blocked on a read.
type stdio struct {
	cmd       *exec.Cmd
	in        io.Writer
	out       io.Reader
	messageID int
	mu        sync.Mutex
}

// newStdIO launches command with args, wires its stdin and stdout, and performs
// the MCP initialize handshake including protocol-version negotiation. The
// child's stderr is discarded and ctx bounds the handshake. On any handshake
// failure the process is terminated before returning.
func newStdIO(ctx context.Context, command string, args []string) (*stdio, error) {
	cmd := exec.Command(command, args...) //nolint:gosec // command and args are operator-provided server config

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening MCP server stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening MCP server stdout: %w", err)
	}

	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fail to start MCP server: %w", err)
	}

	s := &stdio{
		cmd: cmd,
		in:  stdin,
		out: stdout,
	}

	if err := s.initialize(ctx); err != nil {
		_ = s.close()
		return nil, err
	}

	return s, nil
}

func (s *stdio) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    clientName,
			"version": clientVersion,
		},
	}

	result, err := s.Request(ctx, "initialize", params)
	if err != nil {
		return err
	}

	version, ok := result["protocolVersion"].(string)
	if !ok {
		return fmt.Errorf("%w: server did not provide protocol version", ErrUnsupportedProtocolVersion)
	}

	if version != protocolVersion {
		return fmt.Errorf("%w: server offered %q, client supports %q", ErrUnsupportedProtocolVersion, version, protocolVersion)
	}

	return s.notify(ctx, "notifications/initialized", nil)
}

// Request sends a JSON-RPC request and blocks until the response matching its id
// is read, returning that response's result. It holds mu for the whole
// send-and-read, so concurrent callers are serialized. ctx is checked before
// sending and between reads (best-effort cancellation); a request blocked on a
// silent server is unblocked by close, not by ctx. It returns the server's error
// for a JSON-RPC error response, or ErrMCPConnectionClosed if the stream ends first.
func (s *stdio) Request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextMessageID()

	message := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  orEmpty(params),
	}

	if err := s.send(ctx, message); err != nil {
		return nil, fmt.Errorf("sending request to MCP server: %w", err)
	}

	return s.read(ctx, id)
}

func (s *stdio) notify(ctx context.Context, method string, params map[string]any) error {
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  orEmpty(params),
	}

	if err := s.send(ctx, notification); err != nil {
		return fmt.Errorf("sending notification to MCP server: %w", err)
	}

	return nil
}

// close shuts the server down: it closes stdin, then escalates through SIGTERM
// and SIGKILL, waiting closeTimeout at each step, and reaps the process. It takes
// no lock, so it can abort a Request blocked reading from a silent server. It is
// a no-op if the process never started or has already exited.
func (s *stdio) close() error {
	if s.cmd == nil || s.cmd.Process == nil || s.cmd.ProcessState != nil {
		return nil
	}

	if closer, ok := s.in.(io.Closer); ok {
		_ = closer.Close()
	}

	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()

	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		select {
		case <-done:
			return nil
		case <-time.After(closeTimeout):
			_ = s.cmd.Process.Signal(sig)
		}
	}

	<-done

	return nil
}

func (s *stdio) nextMessageID() int {
	s.messageID++
	return s.messageID
}

func (s *stdio) send(ctx context.Context, message map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshaling message to JSON: %w", err)
	}

	if _, err := s.in.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing message to MCP server: %w", err)
	}

	return nil
}

func (s *stdio) read(ctx context.Context, requestID int) (map[string]any, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line, err := readBytes(ctx, s.out, '\n')

		var message map[string]any
		if json.Unmarshal(line, &message) == nil && message["id"] == float64(requestID) {
			if rpcErr, ok := message["error"]; ok {
				return nil, fmt.Errorf("MCP server error: %s", errorMessage(rpcErr))
			}

			if result, ok := message["result"].(map[string]any); ok {
				return result, nil
			}

			return map[string]any{}, nil
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrMCPConnectionClosed
			}

			return nil, fmt.Errorf("reading from MCP server: %w", err)
		}
	}
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
