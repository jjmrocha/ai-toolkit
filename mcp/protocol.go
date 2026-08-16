package mcp

import (
	"fmt"

	"github.com/jjmrocha/go-algo/sets"
)

const (
	protocolVersion = "2025-06-18"
	clientName      = "ai-toolkit"
	clientTitle     = "AI Toolkit"
	clientVersion   = "0.1.0"
)

var supportedVersions = sets.New(protocolVersion, "2025-03-26", "2024-11-05")

func initializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    clientName,
			"title":   clientTitle,
			"version": clientVersion,
		},
	}
}

func acceptProtocolVersion(result map[string]any) error {
	version, ok := result["protocolVersion"].(string)
	if !ok {
		return fmt.Errorf("%w: server did not provide protocol version", ErrUnsupportedProtocolVersion)
	}

	if !supportedVersions.Contains(version) {
		return fmt.Errorf("%w: server offered %q, client offered %q", ErrUnsupportedProtocolVersion, version, protocolVersion)
	}

	return nil
}

func canServe(method string) bool {
	return method == "ping"
}

func canCancel(method string) bool {
	return method != "initialize"
}
