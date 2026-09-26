// Command mcpserver is a tiny MCP server over stdio for the app's tests and the e2e run: two
// tools (echo, add), one resource (note://hello), one prompt (greet). JSON-RPC 2.0, one
// message per line.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type msg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 16<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for in.Scan() {
		var m msg
		if json.Unmarshal(in.Bytes(), &m) != nil {
			continue
		}
		if len(m.ID) == 0 {
			continue // a notification
		}
		reply := msg{JSONRPC: "2.0", ID: m.ID}
		switch m.Method {
		case "initialize":
			reply.Result = map[string]interface{}{"protocolVersion": "2025-06-18", "capabilities": map[string]interface{}{"tools": map[string]interface{}{}, "resources": map[string]interface{}{}, "prompts": map[string]interface{}{}}, "serverInfo": map[string]string{"name": "mcpserver", "version": "1"}}
		case "ping":
			reply.Result = map[string]interface{}{}
		case "tools/list":
			reply.Result = map[string]interface{}{"tools": []interface{}{
				map[string]interface{}{"name": "echo", "description": "echo text back", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"text": map[string]string{"type": "string"}}, "required": []string{"text"}}, "annotations": map[string]interface{}{"readOnlyHint": true}},
				map[string]interface{}{"name": "add", "description": "add two integers", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"a": map[string]string{"type": "integer"}, "b": map[string]string{"type": "integer"}}}},
				map[string]interface{}{"name": "wipe", "description": "destructive by annotation", "inputSchema": map[string]interface{}{"type": "object"}, "annotations": map[string]interface{}{"destructiveHint": true}},
			}}
		case "tools/call":
			var p struct {
				Name string                 `json:"name"`
				Args map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(m.Params, &p)
			text := ""
			isErr := false
			switch p.Name {
			case "echo":
				text, _ = p.Args["text"].(string)
				text = "echo: " + text + " (env " + os.Getenv("MCP_PROBE") + ")"
			case "add":
				a, _ := p.Args["a"].(float64)
				b, _ := p.Args["b"].(float64)
				text = fmt.Sprint(int(a + b))
			case "wipe":
				text = "wiped"
			default:
				text, isErr = "no such tool "+p.Name, true
			}
			reply.Result = map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": text}}, "isError": isErr}
		case "resources/list":
			reply.Result = map[string]interface{}{"resources": []interface{}{map[string]string{"uri": "note://hello", "name": "hello", "mimeType": "text/plain"}}}
		case "resources/read":
			var p struct {
				URI string `json:"uri"`
			}
			_ = json.Unmarshal(m.Params, &p)
			if p.URI != "note://hello" {
				reply.Error = &rpcErr{Code: -32002, Message: "no resource " + p.URI}
			} else {
				reply.Result = map[string]interface{}{"contents": []interface{}{map[string]string{"uri": p.URI, "mimeType": "text/plain", "text": "hello from the resource"}}}
			}
		case "prompts/list":
			reply.Result = map[string]interface{}{"prompts": []interface{}{map[string]interface{}{"name": "greet", "description": "greet someone", "arguments": []interface{}{map[string]interface{}{"name": "who", "required": true}}}}}
		case "prompts/get":
			var p struct {
				Name string            `json:"name"`
				Args map[string]string `json:"arguments"`
			}
			_ = json.Unmarshal(m.Params, &p)
			reply.Result = map[string]interface{}{"messages": []interface{}{map[string]interface{}{"role": "user", "content": map[string]string{"type": "text", "text": "Greet " + p.Args["who"] + " warmly."}}}}
		default:
			reply.Error = &rpcErr{Code: -32601, Message: "method not found: " + m.Method}
		}
		b, _ := json.Marshal(reply)
		out.Write(append(b, '\n'))
		out.Flush()
	}
}
