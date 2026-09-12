package mcpcontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"runweave/internal/contract"
)

type Schema map[string]any
type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Input       Schema `json:"inputSchema"`
	Output      Schema `json:"outputSchema"`
}

const idPattern = `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`

func enum(values ...string) []string { return values }
func subset(base Schema, names ...string) Schema {
	p := map[string]any{}
	all := base["properties"].(map[string]any)
	for _, n := range names {
		p[n] = all[n]
	}
	return Schema{"type": "object", "properties": p, "additionalProperties": false}
}
func schema(t reflect.Type) Schema {
	if t.Kind() == reflect.Pointer {
		return schema(t.Elem())
	}
	s := Schema{}
	switch t.Kind() {
	case reflect.String:
		s = Schema{"type": "string", "maxLength": 4096}
	case reflect.Bool:
		s = Schema{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		s = Schema{"type": "integer", "minimum": 0, "maximum": contract.MaxSafeInteger}
	case reflect.Slice:
		s = Schema{"type": "array", "items": schema(t.Elem()), "maxItems": 1000}
	case reflect.Struct:
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			child := schema(f.Type)
			fieldLimits(name, child)
			props[name] = child
			if f.Tag.Get("optional") != "true" {
				required = append(required, name)
			}
		}
		s = Schema{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	default:
		panic("unsupported schema type: " + t.String())
	}
	switch t {
	case reflect.TypeFor[contract.State]():
		s["enum"] = enum("CREATED", "DISPATCHING", "WAITING_APPROVAL", "RUNNING", "UNKNOWN", "SUCCEEDED", "FAILED", "REJECTED", "CANCELLED", "TIMED_OUT", "EXPIRED")
	case reflect.TypeFor[contract.StartFact]():
		s["enum"] = enum("not_started", "started", "uncertain")
	case reflect.TypeFor[contract.Operation]():
		branches := []Schema{}
		for _, op := range []string{contract.FSList, contract.FSRead, contract.ProcessExec} {
			var b Schema
			switch op {
			case contract.FSList:
				b = subset(s, "type", "target", "max_entries")
				b["required"] = []string{"type", "target"}
			case contract.FSRead:
				b = subset(s, "type", "target", "offset", "limit")
				b["required"] = []string{"type", "target"}
			case contract.ProcessExec:
				b = subset(s, "type", "program", "cwd", "args")
				b["required"] = []string{"type", "program", "cwd"}
			}
			b["properties"].(map[string]any)["type"] = Schema{"type": "string", "enum": []string{op}}
			branches = append(branches, b)
		}
		s = Schema{"type": "object", "oneOf": branches}
	case reflect.TypeFor[contract.Argument]():
		s["oneOf"] = []Schema{{"required": []string{"literal"}}, {"required": []string{"path"}}}
	case reflect.TypeFor[GetInput]():
		s["oneOf"] = []Schema{{"required": []string{"execution_id"}}, {"required": []string{"request_id"}}}
	case reflect.TypeFor[PageInput](), reflect.TypeFor[ResourcesInput]():
		s["properties"].(map[string]any)["limit"] = Schema{"type": "integer", "minimum": 1, "maximum": 64, "default": 64}
	case reflect.TypeFor[contract.Requirements]():
		p := s["properties"].(map[string]any)
		p["os"] = Schema{"type": "string", "enum": []string{"linux"}, "default": "linux"}
		p["resources"].(Schema)["minItems"] = 1
	case reflect.TypeFor[contract.Result]():
		branches := []Schema{}
		for _, op := range []string{contract.FSList, contract.FSRead, contract.ProcessExec} {
			body := map[string]string{contract.FSList: "fs_list", contract.FSRead: "fs_read", contract.ProcessExec: "process"}[op]
			b := subset(s, "operation", "duration_ms", "resources", "mutations", "artifact_refs", "error", body)
			b["required"] = []string{"operation", "duration_ms", "resources", "artifact_refs"}
			b["properties"].(map[string]any)["operation"] = Schema{"type": "string", "enum": []string{op}}
			b["anyOf"] = []Schema{{"required": []string{"error"}}, {"required": []string{body}}}
			branches = append(branches, b)
		}
		s = Schema{"type": "object", "oneOf": branches}
	case reflect.TypeFor[ArtifactChunk]():
		s["properties"].(map[string]any)["encoding"] = Schema{"type": "string", "enum": []string{"base64"}}
	case reflect.TypeFor[contract.Revision]():
		branches := []Schema{}
		for _, kind := range []string{"git_commit", "owner_revision", "sha256"} {
			value := Schema{"type": "string", "pattern": idPattern}
			if kind == "git_commit" {
				value["pattern"] = `^([0-9a-f]{40}|[0-9a-f]{64})$`
			}
			if kind == "sha256" {
				value["pattern"] = `^[0-9a-f]{64}$`
			}
			branches = append(branches, Schema{"type": "object", "required": []string{"type", "value"}, "properties": map[string]any{"type": Schema{"type": "string", "enum": []string{kind}}, "value": value}, "additionalProperties": false})
		}
		s = Schema{"type": "object", "oneOf": branches}
	}
	if strings.HasPrefix(t.Name(), "Reply[") {
		s["oneOf"] = []Schema{{"required": []string{"data"}}, {"required": []string{"error"}}}
	}
	return s
}
func fieldLimits(name string, s Schema) {
	switch name {
	case "request_id", "correlation_id", "execution_id", "node_id", "selected_node_id", "preferred_node", "artifact_id", "approval_id", "program", "rule_version", "reason_code":
		s["pattern"] = idPattern
		s["maxLength"] = 128
		s["minLength"] = 1
	case "request_digest":
		s["pattern"] = `^sha256:[0-9a-f]{64}$`
		s["maxLength"] = 71
	case "cursor", "next_cursor":
		s["minLength"] = 1
		s["maxLength"] = 256
	case "resource":
		s["pattern"] = `^(repo|dataset)://[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`
		s["maxLength"] = 138
	case "relative_path":
		s["minLength"] = 1
		s["description"] = "Resource-relative slash path; root is '.', no absolute path, dot segments, backslash, colon or controls. At most 4096 UTF-8 bytes."
	case "literal":
		s["description"] = "One literal argv item, unchanged; no shell or URI substitution. No NUL; at most 4096 UTF-8 bytes."
	case "timeout_ms":
		s["minimum"] = 1
		s["maximum"] = contract.MaxTimeoutMS
		s["default"] = contract.DefaultTimeoutMS
	case "limit":
		s["minimum"] = 1
		s["maximum"] = 65536
		s["default"] = 65536
	case "offset":
		s["default"] = 0
	case "max_entries":
		s["minimum"] = 1
		s["maximum"] = 1000
		s["default"] = 1000
	case "poll_after_ms":
		s["maximum"] = 30000
	case "config_version", "policy_version", "mapping_version":
		s["minimum"] = 1
	case "exit_code":
		s["maximum"] = 255
	case "bytes_returned":
		s["maximum"] = 65536
	case "capabilities":
		s["maxItems"] = 32
		s["items"] = Schema{"type": "string", "pattern": idPattern}
	case "resources", "nodes", "locations", "candidates":
		s["maxItems"] = 64
	case "artifact_refs":
		s["maxItems"] = 16
	case "args":
		s["maxItems"] = 256
	case "name":
		s["maxLength"] = 255
		s["minLength"] = 1
	case "message":
		s["maxLength"] = 1024
		s["minLength"] = 1
	case "data":
		if s["type"] == "string" {
			s["maxLength"] = 87384
		}
	case "kind":
		s["enum"] = enum("file", "directory", "symlink", "other")
	case "coverage":
		s["enum"] = enum("unknown", "best_effort")
	case "encoding":
		s["enum"] = enum("utf8", "base64")
	case "source":
		s["enum"] = enum("unknown", "git", "owner_manifest", "content_digest")
	case "media_type":
		s["maxLength"] = 128
		s["minLength"] = 1
	case "created_at", "updated_at", "started_at", "finished_at", "expires_at":
		s["maxLength"] = 64
		s["format"] = "date-time"
	}
}
func spec[I, O any](name, description string) ToolSpec {
	return ToolSpec{name, description, schema(reflect.TypeFor[I]()), schema(reflect.TypeFor[Reply[O]]())}
}
func Tools() []ToolSpec {
	return []ToolSpec{
		spec[PageInput, NodesPage]("runweave.list_nodes", "List authorized nodes and scheduling availability; bounded page."),
		spec[ResourcesInput, ResourcesPage]("runweave.list_resources", "List logical resources and authorized locations/revision evidence, never physical roots."),
		spec[contract.Request, ExecutionView]("runweave.execute", "Submit one explicit operation and quickly return its execution view. Reuse request_id after uncertain replies. process.exec may require local approval. args are literal/path objects."),
		spec[GetInput, ExecutionView]("runweave.get_execution", "Query by exactly one execution_id or caller-scoped request_id. Poll according to poll_after_ms; UNKNOWN must not be retried under a new key."),
		spec[CancelInput, ExecutionView]("runweave.cancel_execution", "Persist cancellation intent. cancel_requested is not proof of stopped work; query until a confirmed outcome."),
		spec[ArtifactInput, ArtifactChunk]("runweave.read_artifact", "Read a bounded, currently authorized artifact chunk by opaque ID. Returns base64, never accepts a file path."),
	}
}
func ValidateSchema(s Schema, data []byte) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var parsed jsonschema.Schema
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return err
	}
	var value any
	if err = json.Unmarshal(data, &value); err != nil {
		return err
	}
	return resolved.Validate(value)
}
func Find(name string) (ToolSpec, error) {
	for _, s := range Tools() {
		if s.Name == name {
			return s, nil
		}
	}
	return ToolSpec{}, fmt.Errorf("unknown tool")
}
