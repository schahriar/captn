package lsp

import "encoding/json"

type response struct {
	result json.RawMessage
	err    *RPCError
}

type incomingMessage struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
	Params json.RawMessage `json:"params"`
}
