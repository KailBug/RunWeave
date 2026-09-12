// Package contractprobe is an in-memory P0 fixture, never an execution backend.
package contractprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"runweave/internal/contract"
	mc "runweave/internal/mcpcontract"
	"runweave/internal/strictjson"
)

const Resource = "repo://p0-fixture"
const Node = "p0-simulated-node"
const Artifact = "p0-artifact"
const Content = "RunWeave P0 内存样例\n"

type Engine struct {
	mu    sync.Mutex
	views map[string]mc.ExecutionView
}

func NewEngine() *Engine { return &Engine{views: map[string]mc.ExecutionView{}} }
func ptr[T any](v T) *T  { return &v }
func evidence() []contract.ResourceEvidence {
	return []contract.ResourceEvidence{{Resource: Resource, Source: "unknown"}}
}
func failure(code contract.Code) (any, bool) {
	return map[string]any{"error": contract.Error{Code: code, Message: "P0 fixture: " + string(code)}}, true
}
func success(v any) (any, bool)       { return map[string]any{"data": v}, false }
func decode(raw []byte, dst any) bool { return strictjson.Decode(raw, dst) == nil }

func (e *Engine) Call(name string, raw []byte) (any, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	spec, err := mc.Find(name)
	if err != nil {
		return failure(contract.InvalidRequest)
	}
	if len(raw) > contract.MaxRequestBytes {
		return failure(contract.RequestTooLarge)
	}
	if mc.ValidateSchema(spec.Input, raw) != nil {
		return failure(contract.InvalidRequest)
	}
	switch name {
	case "runweave.list_nodes":
		var in mc.PageInput
		if !decode(raw, &in) || in.Cursor != nil {
			return failure(contract.InvalidRequest)
		}
		return success(mc.NodesPage{Nodes: []mc.NodeSummary{{NodeID: Node, OS: "linux", Capabilities: []string{"fs.list", "fs.read", "process.exec", "python"}, Online: true, Schedulable: true, ConfigVersion: 1}}})
	case "runweave.list_resources":
		var in mc.ResourcesInput
		if !decode(raw, &in) || in.Cursor != nil {
			return failure(contract.InvalidRequest)
		}
		out := mc.ResourcesPage{Resources: []mc.ResourceSummary{}}
		if in.NodeID == nil || *in.NodeID == Node {
			out.Resources = append(out.Resources, mc.ResourceSummary{Resource: Resource, Locations: []mc.Location{{NodeID: Node, Available: true, ReturnContent: true, Evidence: evidence()[0]}}})
		}
		return success(out)
	case "runweave.execute":
		r, err := contract.DecodeRequest(raw)
		if err != nil {
			if ce, ok := err.(*contract.Error); ok {
				return failure(ce.Code)
			}
			return failure(contract.InvalidRequest)
		}
		digest, err := contract.RequestDigest(r)
		if err != nil {
			return failure(contract.InvalidRequest)
		}
		if old, ok := e.views[r.RequestID]; ok {
			if old.RequestDigest != digest {
				return failure(contract.IdempotencyConflict)
			}
			return success(old)
		}
		if r.Requirements.NodeID != nil && *r.Requirements.NodeID != Node {
			return failure(contract.NoMatchingNode)
		}
		for _, cap := range r.Requirements.Capabilities {
			if cap != "fs.list" && cap != "fs.read" && cap != "process.exec" && cap != "python" {
				return failure(contract.NoMatchingNode)
			}
		}
		for _, res := range r.Requirements.Resources {
			if res.Resource != Resource || res.Revision != nil {
				return failure(contract.NoMatchingNode)
			}
		}
		if r.Operation.Target != nil && ((r.Operation.Type == contract.FSList && r.Operation.Target.RelativePath != ".") || (r.Operation.Type == contract.FSRead && r.Operation.Target.RelativePath != "sample.txt")) {
			return failure(contract.ResourceUnavailable)
		}
		if r.Operation.Type == contract.ProcessExec && (*r.Operation.Program != "python" || r.Operation.Cwd.RelativePath != ".") {
			return failure(contract.ProgramUnavailable)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		v := mc.ExecutionView{ExecutionID: fmt.Sprintf("p0-execution-%d", len(e.views)+1), RequestID: r.RequestID, CorrelationID: r.CorrelationID, RequestDigest: digest, NodeID: ptr(Node), CreatedAt: now, UpdatedAt: now, Placement: &mc.Placement{RuleVersion: "p0-fixture-v1", SelectedNodeID: Node, Candidates: []mc.Candidate{{NodeID: Node, Eligible: true, ReasonCode: "fixture_match"}}}}
		if r.Operation.Type == contract.ProcessExec {
			v.State = contract.WaitingApproval
			v.PollAfterMS = 100
			v.Approval = &contract.ApprovalInfo{ApprovalID: "p0-approval-" + v.ExecutionID, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), PolicyVersion: 1, MappingVersion: 1}
		} else {
			result := contract.Result{Operation: r.Operation.Type, Resources: evidence(), ArtifactRefs: []contract.ArtifactRef{{ArtifactID: Artifact, MediaType: "text/plain", SizeBytes: int64(len(Content)), ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}}}
			if r.Operation.Type == contract.FSList {
				result.FSList = &contract.ListResult{Entries: []contract.Entry{{Name: "sample.txt", Kind: "file", SizeBytes: ptr(int64(len(Content)))}}}
			} else {
				chunk := read(*r.Operation.Offset, *r.Operation.Limit)
				result.FSRead = &contract.ReadResult{Offset: chunk.Offset, Encoding: chunk.Encoding, Data: chunk.Data, BytesReturned: chunk.BytesReturned, EOF: chunk.EOF}
			}
			v.State = contract.Succeeded
			v.StartedAt = &now
			v.FinishedAt = &now
			v.Outcome = &contract.Outcome{State: v.State, Start: contract.Started, Quiescent: true, Result: result}
		}
		if mc.ValidateView(v) != nil {
			return failure(contract.InternalError)
		}
		e.views[r.RequestID] = v
		return success(v)
	case "runweave.get_execution", "runweave.cancel_execution":
		var id, requestID string
		if name == "runweave.get_execution" {
			var in mc.GetInput
			if !decode(raw, &in) {
				return failure(contract.InvalidRequest)
			}
			if in.ExecutionID != nil {
				id = *in.ExecutionID
			}
			if in.RequestID != nil {
				requestID = *in.RequestID
			}
		} else {
			var in mc.CancelInput
			if !decode(raw, &in) {
				return failure(contract.InvalidRequest)
			}
			id = in.ExecutionID
		}
		for key, v := range e.views {
			if (id != "" && v.ExecutionID == id) || (requestID != "" && key == requestID) {
				if !v.State.Terminal() {
					v.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
					if name == "runweave.cancel_execution" {
						v.CancelRequested = true
					} else if v.CancelRequested {
						v.State = contract.StateCancelled
						v.Approval = nil
						v.PollAfterMS = 0
						v.FinishedAt = ptr(v.UpdatedAt)
						v.Outcome = &contract.Outcome{State: v.State, Start: contract.NotStarted, Quiescent: true, Result: contract.Result{Operation: contract.ProcessExec, Resources: evidence(), ArtifactRefs: []contract.ArtifactRef{}, Error: &contract.Error{Code: contract.Cancelled, Message: "P0 simulated cancellation confirmed; no process started"}}}
					}
					if mc.ValidateView(v) != nil {
						return failure(contract.InternalError)
					}
					e.views[key] = v
				}
				return success(v)
			}
		}
		return failure(contract.ExecutionNotFound)
	case "runweave.read_artifact":
		var in mc.ArtifactInput
		if !decode(raw, &in) {
			return failure(contract.InvalidRequest)
		}
		if in.ArtifactID != Artifact {
			return failure(contract.ArtifactNotFound)
		}
		offset, limit := int64(0), int64(65536)
		if in.Offset != nil {
			offset = *in.Offset
		}
		if in.Limit != nil {
			limit = *in.Limit
		}
		return success(read(offset, limit))
	}
	return failure(contract.InvalidRequest)
}
func read(offset, limit int64) mc.ArtifactChunk {
	start := min(offset, int64(len(Content)))
	end := min(start+limit, int64(len(Content)))
	return mc.ArtifactChunk{ArtifactID: Artifact, Offset: offset, Encoding: "base64", Data: base64.StdEncoding.EncodeToString([]byte(Content)[start:end]), BytesReturned: end - start, EOF: end == int64(len(Content))}
}

// NewServer only exposes synthetic in-memory fixtures. Audit records protocol data, never auth.
func NewServer(audit io.Writer) *mcp.Server {
	e := NewEngine()
	s := mcp.NewServer(&mcp.Implementation{Name: "runweave-p0-contract-probe", Version: "0.0.1-dev"}, &mcp.ServerOptions{Instructions: "P0 CONTRACT TEST ONLY. Every node, resource, approval, execution and artifact is an in-memory simulation. No files are read and no program is executed. Do not represent these results as real Node execution."})
	for _, spec := range mc.Tools() {
		s.AddTool(&mcp.Tool{Name: spec.Name, Description: "P0 in-memory fixture only. " + spec.Description, InputSchema: spec.Input, OutputSchema: spec.Output, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: spec.Name != "runweave.execute" && spec.Name != "runweave.cancel_execution", IdempotentHint: true, DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, isError := e.Call(spec.Name, req.Params.Arguments)
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, err
			}
			if err = mc.ValidateSchema(spec.Output, raw); err != nil {
				return nil, fmt.Errorf("fixture output schema: %w", err)
			}
			return &mcp.CallToolResult{StructuredContent: out, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, IsError: isError}, nil
		})
	}
	if audit != nil {
		var mu sync.Mutex
		s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				result, err := next(ctx, method, req)
				if method == "initialize" || method == "tools/list" || method == "tools/call" {
					entry := map[string]any{"method": method, "params": req.GetParams(), "result": result}
					if err != nil {
						entry["error"] = err.Error()
					}
					mu.Lock()
					writeErr := json.NewEncoder(audit).Encode(entry)
					mu.Unlock()
					if writeErr != nil {
						return nil, writeErr
					}
				}
				return result, err
			}
		})
	}
	return s
}
