package contractprobe

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"runweave/internal/contract"
	mc "runweave/internal/mcpcontract"
)

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var values []T
	for scanner.Scan() {
		var v T
		if err = json.Unmarshal(scanner.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
func TestRecordedCodexEvidence(t *testing.T) {
	type result struct {
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
		Tools      []mc.ToolSpec   `json:"tools"`
		Protocol   string          `json:"protocolVersion"`
	}
	type audit struct {
		Method string `json:"method"`
		Params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Protocol  string          `json:"protocolVersion"`
		} `json:"params"`
		Result result `json:"result"`
	}
	records := readJSONL[audit](t, "../../docs/资源/MCP契约/server-audit.jsonl")
	type event struct {
		Type string `json:"type"`
		Item struct {
			Type      string          `json:"type"`
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Status    string          `json:"status"`
			Error     any             `json:"error"`
			Result    struct {
				Structured json.RawMessage `json:"structured_content"`
				Content    []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		} `json:"item"`
	}
	events := readJSONL[event](t, "../../docs/资源/MCP契约/codex-calls.jsonl")
	var calls []audit
	initialized, listed := false, false
	equalJSON := func(a, b []byte) bool {
		var x, y any
		return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
	}
	for _, r := range records {
		switch r.Method {
		case "initialize":
			initialized = r.Params.Protocol == "2025-06-18" && r.Result.Protocol == "2025-06-18"
		case "tools/list":
			if len(r.Result.Tools) != 6 {
				t.Fatal("wrong tool count")
			}
			seen := map[string]bool{}
			for _, actual := range r.Result.Tools {
				want, err := mc.Find(actual.Name)
				if err != nil {
					t.Fatal(err)
				}
				if seen[actual.Name] {
					t.Fatal("duplicate tool")
				}
				seen[actual.Name] = true
				for _, pair := range [][2]mc.Schema{{actual.Input, want.Input}, {actual.Output, want.Output}} {
					a, _ := json.Marshal(pair[0])
					b, _ := json.Marshal(pair[1])
					if !equalJSON(a, b) {
						t.Fatalf("recorded schema drift: %s", actual.Name)
					}
				}
			}
			listed = true
		case "tools/call":
			calls = append(calls, r)
		}
	}
	if !initialized || !listed || len(calls) != 13 || len(events) != 13 {
		t.Fatal("incomplete actual client evidence")
	}
	expected := []string{"list_nodes", "list_resources", "execute", "execute", "execute", "execute", "execute", "get_execution", "cancel_execution", "get_execution", "read_artifact", "get_execution", "read_artifact"}
	var views []mc.ExecutionView
	for i, c := range calls {
		ev := events[i]
		if c.Params.Name != "runweave."+expected[i] || ev.Type != "item.completed" || ev.Item.Type != "mcp_tool_call" || ev.Item.Server != "runweave_contract" || ev.Item.Tool != c.Params.Name || ev.Item.Error != nil || !equalJSON(c.Params.Arguments, ev.Item.Arguments) || !equalJSON(c.Result.Structured, ev.Item.Result.Structured) {
			t.Fatalf("client/server mismatch at %d", i+1)
		}
		if len(ev.Item.Result.Content) != 1 || !equalJSON([]byte(ev.Item.Result.Content[0].Text), c.Result.Structured) {
			t.Fatal("text/structured mismatch")
		}
		spec, _ := mc.Find(c.Params.Name)
		if err := mc.ValidateSchema(spec.Input, c.Params.Arguments); err != nil {
			t.Fatal(err)
		}
		if err := mc.ValidateSchema(spec.Output, c.Result.Structured); err != nil {
			t.Fatal(err)
		}
		code := map[int]string{4: "IDEMPOTENCY_CONFLICT", 11: "EXECUTION_NOT_FOUND", 12: "ARTIFACT_NOT_FOUND"}[i]
		if c.Result.IsError != (code != "") || (ev.Item.Status == "failed") != (code != "") || (code == "" && ev.Item.Status != "completed") {
			t.Fatal("wrong error semantics")
		}
		if code != "" {
			var reply mc.Reply[any]
			if err := json.Unmarshal(c.Result.Structured, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Error == nil || string(reply.Error.Code) != code {
				t.Fatal("wrong code")
			}
			continue
		}
		if i >= 2 && i <= 9 {
			var reply mc.Reply[mc.ExecutionView]
			if err := json.Unmarshal(c.Result.Structured, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Data == nil {
				t.Fatal("missing execution")
			}
			if err := mc.ValidateView(*reply.Data); err != nil {
				t.Fatal(err)
			}
			views = append(views, *reply.Data)
			if c.Params.Name == "runweave.execute" {
				r, err := contract.DecodeRequest(c.Params.Arguments)
				if err != nil {
					t.Fatal(err)
				}
				digest, err := contract.RequestDigest(r)
				if err != nil || digest != reply.Data.RequestDigest {
					t.Fatal("request digest mismatch")
				}
			}
		}
	}
	if len(views) != 7 || views[0].ExecutionID != views[1].ExecutionID || views[0].RequestDigest != views[1].RequestDigest || views[2].Outcome.Result.FSRead.Data != "UnVuVw==" || views[3].State != contract.WaitingApproval || views[4].State != contract.WaitingApproval || views[5].State != contract.WaitingApproval || !views[5].CancelRequested || views[6].State != contract.StateCancelled || views[6].Outcome.Start != contract.NotStarted {
		t.Fatal("wrong business scenario")
	}
	var artifact mc.Reply[mc.ArtifactChunk]
	if err := json.Unmarshal(calls[10].Result.Structured, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Data == nil || artifact.Data.ArtifactID != views[0].Outcome.Result.ArtifactRefs[0].ArtifactID || artifact.Data.Data != "UnVuVw==" || artifact.Data.BytesReturned != 4 || artifact.Data.EOF {
		t.Fatal("wrong artifact")
	}
	// The same client-visible validator must reject contradictory terminal evidence.
	bad := views[6]
	bad.Outcome = nil
	if mc.ValidateView(bad) == nil {
		t.Fatal("accepted terminal without evidence")
	}
	bad = views[5]
	bad.FinishedAt = &bad.UpdatedAt
	if mc.ValidateView(bad) == nil {
		t.Fatal("accepted unconfirmed cancellation as finished")
	}
	bad = views[3]
	bad.Approval = nil
	if mc.ValidateView(bad) == nil {
		t.Fatal("accepted waiting without approval")
	}
}
