package mcp

// Status reports whether a registered MCP is currently running, as returned by
// [Manager.Status].
type Status struct {
	// Name is the MCP's registered name.
	Name string
	// Active is true while the MCP's server process is running.
	Active bool
}

type toolSpec struct {
	name        string
	description string
	schema      map[string]any
}

type request struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

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
