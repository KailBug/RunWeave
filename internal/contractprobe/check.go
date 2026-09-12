package contractprobe

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mc "runweave/internal/mcpcontract"
)

// Verify exercises real tool responses, not the client's natural-language report.
func Verify(ctx context.Context, session *mcp.ClientSession) error {
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	if len(list.Tools) != 6 {
		return fmt.Errorf("expected six tools")
	}
	for _, t := range list.Tools {
		spec, err := mc.Find(t.Name)
		if err != nil {
			return err
		}
		for _, pair := range [][2]any{{t.InputSchema, spec.Input}, {t.OutputSchema, spec.Output}} {
			a, _ := json.Marshal(pair[0])
			b, _ := json.Marshal(pair[1])
			if string(a) != string(b) {
				return fmt.Errorf("schema mismatch: %s", t.Name)
			}
		}
	}
	call := func(name, args, code string) (map[string]any, error) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "runweave." + name, Arguments: json.RawMessage(args)})
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return nil, err
		}
		spec, _ := mc.Find("runweave." + name)
		if err := mc.ValidateSchema(spec.Output, raw); err != nil {
			return nil, err
		}
		if result.IsError != (code != "") {
			return nil, fmt.Errorf("%s unexpected error: %s", name, raw)
		}
		if len(result.Content) != 1 {
			return nil, fmt.Errorf("missing compatibility text")
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		var textValue any
		if !ok {
			return nil, fmt.Errorf("missing text content")
		}
		if err := json.Unmarshal([]byte(text.Text), &textValue); err != nil {
			return nil, err
		}
		canonicalText, _ := json.Marshal(textValue)
		if string(canonicalText) != string(raw) {
			return nil, fmt.Errorf("text/structured mismatch")
		}
		var out map[string]any
		if err = json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		if code != "" {
			if out["error"].(map[string]any)["code"] != code {
				return nil, fmt.Errorf("unexpected error code: %s", raw)
			}
			return nil, nil
		}
		data := out["data"].(map[string]any)
		if name == "execute" || name == "get_execution" || name == "cancel_execution" {
			var v mc.ExecutionView
			b, _ := json.Marshal(data)
			if err = json.Unmarshal(b, &v); err != nil {
				return nil, err
			}
			if err = mc.ValidateView(v); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	if _, err = call("list_nodes", `{}`, ""); err != nil {
		return err
	}
	if _, err = call("list_resources", `{"node_id":"p0-simulated-node","limit":1}`, ""); err != nil {
		return err
	}
	const req = `{"request_id":"stdio-list","requirements":{"resources":[{"resource":"repo://p0-fixture"}]},"operation":{"type":"fs.list","target":{"resource":"repo://p0-fixture","relative_path":"."}}}`
	first, err := call("execute", req, "")
	if err != nil {
		return err
	}
	second, err := call("execute", req, "")
	if err != nil {
		return err
	}
	if first["execution_id"] != second["execution_id"] {
		return fmt.Errorf("idempotency failed")
	}
	if _, err = call("execute", `{"request_id":"stdio-list","requirements":{"resources":[{"resource":"repo://p0-fixture"}]},"operation":{"type":"fs.read","target":{"resource":"repo://p0-fixture","relative_path":"sample.txt"}}}`, "IDEMPOTENCY_CONFLICT"); err != nil {
		return err
	}
	if _, err = call("execute", `{"request_id":"stdio-read","requirements":{"resources":[{"resource":"repo://p0-fixture"}]},"operation":{"type":"fs.read","target":{"resource":"repo://p0-fixture","relative_path":"sample.txt"},"offset":0,"limit":4}}`, ""); err != nil {
		return err
	}
	process, err := call("execute", `{"request_id":"stdio-process","requirements":{"resources":[{"resource":"repo://p0-fixture"}]},"operation":{"type":"process.exec","program":"python","cwd":{"resource":"repo://p0-fixture","relative_path":"."},"args":[{"literal":"--version"},{"path":{"resource":"repo://p0-fixture","relative_path":"sample.txt"}}]}}`, "")
	if err != nil {
		return err
	}
	if process["state"] != "WAITING_APPROVAL" {
		return fmt.Errorf("missing approval")
	}
	if _, err = call("get_execution", `{"request_id":"stdio-process"}`, ""); err != nil {
		return err
	}
	args := fmt.Sprintf(`{"execution_id":%q}`, process["execution_id"])
	pending, err := call("cancel_execution", args, "")
	if err != nil {
		return err
	}
	if pending["state"] != "WAITING_APPROVAL" || pending["cancel_requested"] != true {
		return fmt.Errorf("cancel must only express intent")
	}
	confirmed, err := call("get_execution", args, "")
	if err != nil {
		return err
	}
	if confirmed["state"] != "CANCELLED" {
		return fmt.Errorf("missing cancellation confirmation")
	}
	chunk, err := call("read_artifact", `{"artifact_id":"p0-artifact","offset":0,"limit":4}`, "")
	if err != nil {
		return err
	}
	if chunk["data"] != "UnVuVw==" || chunk["bytes_returned"] != float64(4) || chunk["eof"] != false {
		return fmt.Errorf("wrong artifact chunk")
	}
	for _, c := range [][3]string{{"get_execution", `{"request_id":"missing"}`, "EXECUTION_NOT_FOUND"}, {"read_artifact", `{"artifact_id":"missing"}`, "ARTIFACT_NOT_FOUND"}, {"list_nodes", `{"limit":0}`, "INVALID_REQUEST"}, {"list_nodes", `{"limit":null}`, "INVALID_REQUEST"}, {"list_nodes", `{"unexpected":1}`, "INVALID_REQUEST"}, {"get_execution", `{"request_id":"x","execution_id":"y"}`, "INVALID_REQUEST"}, {"list_nodes", `{"limit":1,"limit":2}`, "INVALID_REQUEST"}} {
		if _, err = call(c[0], c[1], c[2]); err != nil {
			return err
		}
	}
	_, err = call("list_nodes", `{}`, "")
	return err
}
