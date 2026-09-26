// fakevllm stands in for a hosted vLLM in the €0 plumbing test: it speaks the OpenAI-compatible
// API (/v1/models with max_model_len, /v1/chat/completions with tool_calls and usage) and plays a
// fixed script, so a Harbor run proves the path end to end without any real model.
//
//	FAKE_MODE=right  → the bash call writes the right answer (the verifier should pass)
//	FAKE_MODE=wrong  → the bash call writes a wrong answer (the verifier should fail)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolDef struct {
	Function struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []toolDef `json:"tools"`
}

var calls atomic.Int64

func main() {
	addr := flag.String("addr", "127.0.0.1:18000", "listen address")
	model := flag.String("model", "fake/vllm-coder", "served model id")
	flag.Parse()
	mode := os.Getenv("FAKE_MODE")
	if mode == "" {
		mode = "right"
	}
	answer := map[string]string{"right": "hello isekai", "wrong": "goodbye"}[mode]
	if answer == "" {
		log.Fatalf("FAKE_MODE must be right or wrong, got %q", mode)
	}

	http.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"object": "list", "data": []any{
			map[string]any{"id": *model, "object": "model", "owned_by": "fake", "max_model_len": 32768},
		}})
	})
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n := calls.Add(1)
		names := make([]string, 0, len(req.Tools))
		for _, t := range req.Tools {
			names = append(names, t.Function.Name)
		}
		toolResults := 0
		for _, m := range req.Messages {
			if m.Role == "tool" {
				toolResults++
			}
		}
		log.Printf("call %d model=%s messages=%d tool_results=%d tools=%s", n, req.Model, len(req.Messages), toolResults, strings.Join(names, ","))

		usage := map[string]any{"prompt_tokens": 900 + 150*len(req.Messages), "completion_tokens": 40, "total_tokens": 940 + 150*len(req.Messages)}
		if toolResults == 0 {
			if !contains(names, "bash") {
				http.Error(w, "fakevllm: no `bash` tool offered; offered: "+strings.Join(names, ","), http.StatusBadRequest)
				return
			}
			args, _ := json.Marshal(map[string]string{"command": fmt.Sprintf("printf '%%s\\n' '%s' > /app/hello.txt && cat /app/hello.txt", answer)})
			writeJSON(w, completion(req.Model, map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{
					"id": fmt.Sprintf("call_%d", n), "type": "function",
					"function": map[string]any{"name": "bash", "arguments": string(args)},
				}},
			}, "tool_calls", usage))
			return
		}
		writeJSON(w, completion(req.Model, map[string]any{
			"role": "assistant", "content": "@S DONE wrote /app/hello.txt\n@E 30",
		}, "stop", usage))
	})
	log.Printf("fakevllm mode=%s model=%s on %s", mode, *model, *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func completion(model string, msg map[string]any, finish string, usage map[string]any) map[string]any {
	return map[string]any{
		"id": fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()), "object": "chat.completion",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}},
		"usage":   usage,
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
