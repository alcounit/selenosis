package jsonrpc

import (
	"encoding/json"
	"net/http"
)

// JSON-RPC 2.0 error codes, see https://www.jsonrpc.org/specification#error_object :
// -32700/-32600/-32603 are reserved (parse error / invalid request / internal error);
// -32000..-32099 are reserved for server-defined errors. SessionNotFound (-32001) matches
// the MCP Streamable HTTP transport in the official SDK
// (https://github.com/modelcontextprotocol/typescript-sdk).
const (
	InvalidRequest  = -32600
	InvalidParams   = -32602
	InternalError   = -32603
	SessionNotFound = -32001
)

// WriteError writes an MCP (JSON-RPC 2.0) error response with the given HTTP status and code.
func WriteError(rw http.ResponseWriter, status, code int, message string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      nil,
		"error":   map[string]any{"code": code, "message": message},
	})
}
