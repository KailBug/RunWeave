// Package protocol defines Node v1 messages, without networking or persistence.
package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"runweave/internal/contract"
	"runweave/internal/strictjson"
)

const Version = 1
const MaxFrameBytes = 1 << 20

type Sender string

const (
	Node   Sender = "node"
	Server Sender = "server"
)

type Context struct {
	Sender Sender
	NodeID string
	Epoch  string
}

type Envelope struct {
	ProtocolVersion int                        `json:"protocol_version"`
	Type            string                     `json:"type"`
	MessageID       string                     `json:"message_id"`
	ConnectionEpoch string                     `json:"connection_epoch"`
	ExecutionID     *string                    `json:"execution_id,omitempty" optional:"true"`
	Payload         json.RawMessage            `json:"payload"`
	Extensions      map[string]json.RawMessage `json:"extensions,omitempty" optional:"true"`
}
type Message struct {
	Envelope Envelope
	Payload  any
}
type ResourceLocation struct {
	Available     bool                      `json:"available"`
	ReturnContent bool                      `json:"return_content"`
	Evidence      contract.ResourceEvidence `json:"evidence"`
}
type Register struct {
	NodeID            string             `json:"node_id"`
	SoftwareVersion   string             `json:"software_version"`
	JournalInstanceID string             `json:"journal_instance_id"`
	ConfigVersion     int64              `json:"config_version"`
	Capabilities      []string           `json:"capabilities"`
	Resources         []ResourceLocation `json:"resources"`
}
type Registered struct {
	NodeID            string `json:"node_id"`
	HeartbeatMS       int64  `json:"heartbeat_ms"`
	OfflineAfterMS    int64  `json:"offline_after_ms"`
	ReconcileRequired bool   `json:"reconcile_required"`
}
type Heartbeat struct {
	ConfigVersion int64 `json:"config_version"`
}
type Dispatch struct {
	RequestDigest string          `json:"request_digest"`
	Request       json.RawMessage `json:"request"`
}
type Observation struct {
	RequestDigest string                `json:"request_digest"`
	Evidence      contract.NodeEvidence `json:"evidence"`
}
type ExecutionRef struct {
	RequestDigest string `json:"request_digest"`
}
type Snapshot struct {
	QueryMessageID string                `json:"query_message_id"`
	RequestDigest  string                `json:"request_digest"`
	Evidence       contract.NodeEvidence `json:"evidence"`
}
type Receipt struct {
	RequestDigest string `json:"request_digest"`
	ResultDigest  string `json:"result_digest"`
}
type ArtifactRequest struct {
	ArtifactID  string `json:"artifact_id"`
	PrincipalID string `json:"principal_id"`
	Offset      int64  `json:"offset"`
	Limit       int64  `json:"limit"`
}
type ArtifactResponse struct {
	RequestMessageID string          `json:"request_message_id"`
	ArtifactID       string          `json:"artifact_id"`
	Offset           int64           `json:"offset"`
	Data             *string         `json:"data,omitempty" optional:"true"`
	BytesReturned    *int64          `json:"bytes_returned,omitempty" optional:"true"`
	EOF              *bool           `json:"eof,omitempty" optional:"true"`
	Error            *contract.Error `json:"error,omitempty" optional:"true"`
}

func positive(v int64) bool { return v > 0 && v <= contract.MaxSafeInteger }
func readRange(offset, limit int64) bool {
	return offset >= 0 && limit > 0 && limit <= contract.MaxContentBytes && offset <= contract.MaxSafeInteger-limit
}
func uniqueIDs(values []string, max int) bool {
	if len(values) > max {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !contract.ValidIdentifier(v) || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func senderFor(kind string) Sender {
	switch kind {
	case "register", "heartbeat", "execution.accepted", "execution.approval_required", "execution.started", "execution.result", "execution.snapshot", "artifact.response":
		return Node
	case "registered", "execution.dispatch", "execution.cancel", "execution.query", "result.receipt", "artifact.request":
		return Server
	}
	return ""
}

func Decode(data []byte, ctx Context) (Message, error) {
	bad := func() (Message, error) { return Message{}, fmt.Errorf("invalid Node v1 message or session context") }
	if len(data) > MaxFrameBytes || !contract.ValidIdentifier(ctx.NodeID) {
		return bad()
	}
	var e Envelope
	if err := strictjson.Decode(data, &e); err != nil {
		return bad()
	}
	if e.ProtocolVersion != Version || senderFor(e.Type) == "" || senderFor(e.Type) != ctx.Sender || !contract.ValidIdentifier(e.MessageID) {
		return bad()
	}
	switch e.Type {
	case "register":
		if ctx.Epoch != "" || e.ConnectionEpoch != "" {
			return bad()
		}
	case "registered":
		if ctx.Epoch != "" || !contract.ValidIdentifier(e.ConnectionEpoch) {
			return bad()
		}
	default:
		if !contract.ValidIdentifier(ctx.Epoch) || e.ConnectionEpoch != ctx.Epoch {
			return bad()
		}
	}
	execution := strings.HasPrefix(e.Type, "execution.") || strings.HasPrefix(e.Type, "artifact.") || e.Type == "result.receipt"
	if execution != (e.ExecutionID != nil) || e.ExecutionID != nil && !contract.ValidIdentifier(*e.ExecutionID) {
		return bad()
	}
	if len(e.Extensions) > 16 {
		return bad()
	}
	for name, value := range e.Extensions {
		encodedValue, err := json.Marshal(value)
		if !contract.ValidIdentifier(name) || len(value) > 4096 || err != nil || len(encodedValue) > 4096 {
			return bad()
		}
	}
	var payload any
	switch e.Type {
	case "register":
		payload = &Register{}
	case "registered":
		payload = &Registered{}
	case "heartbeat":
		payload = &Heartbeat{}
	case "execution.dispatch":
		payload = &Dispatch{}
	case "execution.accepted", "execution.approval_required", "execution.started", "execution.result":
		payload = &Observation{}
	case "execution.cancel", "execution.query":
		payload = &ExecutionRef{}
	case "execution.snapshot":
		payload = &Snapshot{}
	case "result.receipt":
		payload = &Receipt{}
	case "artifact.request":
		payload = &ArtifactRequest{}
	case "artifact.response":
		payload = &ArtifactResponse{}
	}
	if err := strictjson.Decode(e.Payload, payload); err != nil {
		return bad()
	}
	if err := validatePayload(e.Type, payload, ctx); err != nil {
		return bad()
	}
	// Recheck full encoding, including typed payload normalization/defaults.
	encoded, err := json.Marshal(payload)
	if err != nil {
		return bad()
	}
	e.Payload = encoded
	encoded, err = json.Marshal(e)
	if err != nil || len(encoded) > MaxFrameBytes {
		return bad()
	}
	return Message{e, payload}, nil
}

// Encode validates the raw envelope payload through the same path as Decode.
// Use json.Marshal on a payload type when constructing an envelope.
func Encode(e Envelope, ctx Context) ([]byte, error) {
	if !strictjson.ValidStrings(e) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	m, err := Decode(data, ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m.Envelope)
}

func validatePayload(kind string, payload any, ctx Context) error {
	bad := func() error { return fmt.Errorf("invalid payload") }
	switch p := payload.(type) {
	case *Register:
		if p.NodeID != ctx.NodeID || !contract.ValidIdentifier(p.SoftwareVersion) || len(p.SoftwareVersion) > 64 || !contract.ValidIdentifier(p.JournalInstanceID) || !positive(p.ConfigVersion) || !uniqueIDs(p.Capabilities, 32) || len(p.Resources) > 64 {
			return bad()
		}
		evidence := []contract.ResourceEvidence{}
		for _, r := range p.Resources {
			evidence = append(evidence, r.Evidence)
		}
		// The same business validator governs resource evidence in registration/results.
		_, err := contract.EncodeResult(contract.Result{Operation: contract.FSList, Resources: evidence, ArtifactRefs: []contract.ArtifactRef{}, FSList: &contract.ListResult{Entries: []contract.Entry{}}})
		if err != nil {
			return bad()
		}
	case *Registered:
		if p.NodeID != ctx.NodeID || p.HeartbeatMS != 10000 || p.OfflineAfterMS != 30000 || !p.ReconcileRequired {
			return bad()
		}
	case *Heartbeat:
		if !positive(p.ConfigVersion) {
			return bad()
		}
	case *Dispatch:
		if !contract.ValidDigest(p.RequestDigest) {
			return bad()
		}
		r, err := contract.DecodeRequest(p.Request)
		if err != nil {
			return bad()
		}
		digest, err := contract.RequestDigest(r)
		if err != nil || digest != p.RequestDigest {
			return bad()
		}
		p.Request, _ = json.Marshal(r)
	case *Observation:
		if !contract.ValidDigest(p.RequestDigest) || contract.ValidateEvidence(p.Evidence) != nil {
			return bad()
		}
		want := map[string]contract.Phase{"execution.accepted": contract.Received, "execution.approval_required": contract.AwaitingApproval, "execution.started": contract.Executing, "execution.result": contract.Finished}[kind]
		if p.Evidence.Phase != want {
			return bad()
		}
	case *ExecutionRef:
		if !contract.ValidDigest(p.RequestDigest) {
			return bad()
		}
	case *Snapshot:
		if !contract.ValidIdentifier(p.QueryMessageID) || !contract.ValidDigest(p.RequestDigest) || contract.ValidateEvidence(p.Evidence) != nil {
			return bad()
		}
	case *Receipt:
		if !contract.ValidDigest(p.RequestDigest) || !contract.ValidDigest(p.ResultDigest) {
			return bad()
		}
	case *ArtifactRequest:
		if !contract.ValidIdentifier(p.ArtifactID) || !contract.ValidIdentifier(p.PrincipalID) || !readRange(p.Offset, p.Limit) {
			return bad()
		}
	case *ArtifactResponse:
		if !contract.ValidIdentifier(p.RequestMessageID) || !contract.ValidIdentifier(p.ArtifactID) || p.Offset < 0 || p.Offset > contract.MaxSafeInteger {
			return bad()
		}
		if p.Error != nil {
			if p.Data != nil || p.BytesReturned != nil || p.EOF != nil || len(p.Error.Message) == 0 || len(p.Error.Message) > 1024 || strings.ContainsRune(p.Error.Message, 0) {
				return bad()
			}
			switch p.Error.Code {
			case contract.ArtifactNotFound, contract.ArtifactUnavailable, contract.ArtifactExpired, contract.Forbidden, contract.IOError:
			default:
				return bad()
			}
		} else {
			if p.Data == nil || p.BytesReturned == nil || p.EOF == nil || *p.BytesReturned < 0 || *p.BytesReturned > contract.MaxContentBytes || p.Offset > contract.MaxSafeInteger-*p.BytesReturned || len(*p.Data) > base64.StdEncoding.EncodedLen(contract.MaxContentBytes) {
				return bad()
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(*p.Data)
			if err != nil || int64(len(decoded)) != *p.BytesReturned || base64.StdEncoding.EncodeToString(decoded) != *p.Data {
				return bad()
			}
		}
	default:
		return bad()
	}
	return nil
}

// MatchReply revalidates values to avoid trusting caller-constructed Message
// payloads. The service must additionally consume a real outstanding request.
func MatchReply(request, response Envelope, nodeID string) error {
	if request.ConnectionEpoch != response.ConnectionEpoch || request.ExecutionID == nil || response.ExecutionID == nil || *request.ExecutionID != *response.ExecutionID {
		return fmt.Errorf("reply identity mismatch")
	}
	leftSender := senderFor(request.Type)
	rightSender := senderFor(response.Type)
	leftRaw, err := Encode(request, Context{leftSender, nodeID, request.ConnectionEpoch})
	if err != nil {
		return err
	}
	rightRaw, err := Encode(response, Context{rightSender, nodeID, response.ConnectionEpoch})
	if err != nil {
		return err
	}
	left, err := Decode(leftRaw, Context{leftSender, nodeID, request.ConnectionEpoch})
	if err != nil {
		return err
	}
	right, err := Decode(rightRaw, Context{rightSender, nodeID, response.ConnectionEpoch})
	if err != nil {
		return err
	}
	switch request.Type {
	case "execution.query":
		p, ok := right.Payload.(*Snapshot)
		if !ok || p.QueryMessageID != request.MessageID || p.RequestDigest != left.Payload.(*ExecutionRef).RequestDigest {
			return fmt.Errorf("snapshot mismatch")
		}
	case "execution.result":
		p, ok := right.Payload.(*Receipt)
		if !ok {
			return fmt.Errorf("receipt type mismatch")
		}
		e := left.Payload.(*Observation)
		digest, err := contract.OutcomeDigest(*e.Evidence.Outcome)
		if err != nil || p.RequestDigest != e.RequestDigest || p.ResultDigest != digest {
			return fmt.Errorf("receipt mismatch")
		}
	case "artifact.request":
		p, ok := right.Payload.(*ArtifactResponse)
		if !ok {
			return fmt.Errorf("artifact reply type mismatch")
		}
		q := left.Payload.(*ArtifactRequest)
		if p.RequestMessageID != request.MessageID || p.ArtifactID != q.ArtifactID || p.Offset != q.Offset || p.BytesReturned != nil && *p.BytesReturned > q.Limit {
			return fmt.Errorf("artifact reply mismatch")
		}
	default:
		return fmt.Errorf("no reply contract for this message")
	}
	return nil
}
