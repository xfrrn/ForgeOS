package store

const ToolWindow = 32

type ToolEvent struct {
	Seq   int64  `json:"seq"`
	Call  int64  `json:"call"`
	Tool  string `json:"tool"`
	Phase string `json:"phase"`
	At    int64  `json:"at"`
}

func KnownTool(harness, tool string) bool {
	switch tool {
	case "command_execution", "file_change", "mcp_tool_call", "web_search":
		return harness == "codex"
	case "Read", "Glob", "Grep", "Edit", "Write":
		return harness == "claude"
	}
	return false
}

func (p HarnessProcess) validTools() bool {
	events := p.Tools
	if len(events) > ToolWindow || len(events) > 0 && (p.Session == "" || (len(events) < ToolWindow && events[0].Seq != 1)) {
		return false
	}
	for i, e := range events {
		if !KnownTool(p.Harness, e.Tool) || (e.Phase != "started" && e.Phase != "completed" && e.Phase != "failed" && e.Phase != "output") ||
			(e.Phase == "output" && (e.Tool != "command_execution" || e.Call >= e.Seq)) ||
			e.At < p.Started.Unix() || e.At > 253402300799 || (p.Exited != nil && e.At > p.Exited.Unix()) || e.Call < 1 || e.Call > e.Seq || (e.Phase == "started" && e.Call != e.Seq) || (i > 0 && e.Seq != events[i-1].Seq+1) {
			return false
		}
	}
	return true
}

func (p HarnessProcess) toolsFollow(last HarnessProcess) bool {
	if len(last.Tools) == 0 {
		return last.Exited == nil || len(p.Tools) == 0
	}
	if len(p.Tools) == 0 {
		return false
	}
	end := p.Tools[len(p.Tools)-1].Seq
	prior := last.Tools[len(last.Tools)-1].Seq
	if end < prior || (last.Exited != nil && end != prior) {
		return false
	}
	for _, e := range p.Tools {
		if e.Seq > prior {
			break
		}
		index := e.Seq - last.Tools[0].Seq
		if index < 0 || last.Tools[index] != e {
			return false
		}
	}
	return true
}
