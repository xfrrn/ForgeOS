package chatkernel

import (
	"context"
	"encoding/json"
	"net/http"
)

const ResolveTask = "resolve_task_reference"
const TaskProgress = "get_task_progress"

type TaskToolCall func(context.Context, string, json.RawMessage) (json.RawMessage, error)

func taskToolDefinition(name, description string, properties obj, required ...string) obj {
	return obj{"name": name, "description": description, "annotations": obj{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false},
		"inputSchema": obj{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}
}

// Stateless streamable HTTP MCP on the same turn-private loopback bridge. The
// node authorization and lease stay in the callback and never reach the model.
func (c *Config) taskTools(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     json.RawMessage
		Method string
		Params struct {
			ProtocolVersion, Name string
			Arguments             json.RawMessage
		}
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid MCP request", 400)
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	response := obj{"jsonrpc": "2.0", "id": req.ID}
	switch req.Method {
	case "initialize":
		version := req.Params.ProtocolVersion
		switch version {
		case "2025-03-26", "2025-06-18", "2025-11-25":
		default:
			version = "2025-06-18"
		}
		response["result"] = obj{"protocolVersion": version, "serverInfo": obj{"name": "forgeos_tasks", "version": "1.0"}, "capabilities": obj{"tools": obj{}}}
	case "ping":
		response["result"] = obj{}
	case "tools/list":
		response["result"] = obj{"tools": []obj{
			taskToolDefinition(ResolveTask, "Locate a task in this project's durable chat history. Use recent for 'that task', latest_dispatch for the last confirmed dispatch, or search by task ID, Issue number, or title fragment. Ask if ambiguous.", obj{"by": obj{"type": "string", "enum": []string{"recent", "latest_dispatch", "task_id", "issue_number", "title"}}, "value": obj{"type": "string", "description": "Digits for an ID or issue number; a literal fragment for a title. Omit for recent/latest_dispatch."}}, "by"),
			taskToolDefinition(TaskProgress, "Read fresh task status, recent event, children, dependencies, outstanding owner questions, delivery and execution health.", obj{"task_id": obj{"type": "integer", "minimum": 1}}, "task_id")}}
	case "tools/call":
		name := req.Params.Name
		if name != ResolveTask && name != TaskProgress {
			response["error"] = obj{"code": -32602, "message": "unknown tool"}
			break
		}
		raw, err := c.TaskTool(r.Context(), name, req.Params.Arguments)
		output := string(raw)
		if err != nil {
			output = err.Error()
		}
		response["result"] = obj{"isError": err != nil, "content": []obj{{"type": "text", "text": output}}}
	default:
		response["error"] = obj{"code": -32601, "message": "method not found"}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
