package mcpcontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runweave/internal/contract"
	"strings"
	"testing"
)

func TestPublishedSchemas(t *testing.T) {
	raw, err := os.ReadFile("../../docs/资源/MCP契约/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var published []ToolSpec
	if err = json.Unmarshal(raw, &published); err != nil {
		t.Fatal(err)
	}
	generated, _ := json.Marshal(Tools())
	stored, _ := json.Marshal(published)
	if string(generated) != string(stored) {
		t.Fatal("published schema is stale")
	}
}
func TestSchemasAcceptBusinessFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../docs/资源/契约/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(filepath.Base(path), "request-") {
				if _, err = contract.DecodeRequest(raw); err == nil {
					s, _ := Find("runweave.execute")
					if err = ValidateSchema(s.Input, raw); err != nil {
						t.Fatal(err)
					}
				}
			}
			if strings.HasPrefix(filepath.Base(path), "result-") {
				if _, err = contract.DecodeResult(raw); err == nil {
					if err = ValidateSchema(schema(reflect.TypeFor[contract.Result]()), raw); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
func TestSchemaRejectsInvalidUnions(t *testing.T) {
	for _, c := range []struct {
		name, raw string
		output    bool
	}{
		{"runweave.get_execution", `{}`, false},
		{"runweave.get_execution", `{"request_id":"a","execution_id":"b"}`, false},
		{"runweave.list_nodes", `{"limit":65}`, false},
		{"runweave.list_nodes", `{"data":{"nodes":[]},"error":{"code":"INVALID_REQUEST","message":"bad"}}`, true},
		{"runweave.list_nodes", `{}`, true},
		{"runweave.list_nodes", `{"data":null}`, true},
		{"runweave.execute", `{"request_id":"a","requirements":{"resources":[{"resource":"repo://demo"}]},"operation":{"type":"fs.list","program":"python","target":{"resource":"repo://demo","relative_path":"."}}}`, false},
	} {
		s, _ := Find(c.name)
		test := s.Input
		if c.output {
			test = s.Output
		}
		if ValidateSchema(test, []byte(c.raw)) == nil {
			t.Errorf("accepted %s", c.raw)
		}
	}
}
