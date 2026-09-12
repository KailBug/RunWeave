// Package mcpcontract defines the client-facing v1 tool contract, not a backend.
package mcpcontract

import (
	"fmt"
	"time"

	"runweave/internal/contract"
)

type PageInput struct {
	Limit  *int    `json:"limit,omitempty" optional:"true"`
	Cursor *string `json:"cursor,omitempty" optional:"true"`
}
type ResourcesInput struct {
	NodeID *string `json:"node_id,omitempty" optional:"true"`
	Limit  *int    `json:"limit,omitempty" optional:"true"`
	Cursor *string `json:"cursor,omitempty" optional:"true"`
}
type GetInput struct {
	ExecutionID *string `json:"execution_id,omitempty" optional:"true"`
	RequestID   *string `json:"request_id,omitempty" optional:"true"`
}
type CancelInput struct {
	ExecutionID string `json:"execution_id"`
}
type ArtifactInput struct {
	ArtifactID string `json:"artifact_id"`
	Offset     *int64 `json:"offset,omitempty" optional:"true"`
	Limit      *int64 `json:"limit,omitempty" optional:"true"`
}
type Reply[T any] struct {
	Data  *T              `json:"data,omitempty" optional:"true"`
	Error *contract.Error `json:"error,omitempty" optional:"true"`
}
type NodeSummary struct {
	NodeID        string   `json:"node_id"`
	OS            string   `json:"os"`
	Capabilities  []string `json:"capabilities"`
	Online        bool     `json:"online"`
	Schedulable   bool     `json:"schedulable"`
	ConfigVersion int64    `json:"config_version"`
}
type NodesPage struct {
	Nodes      []NodeSummary `json:"nodes"`
	NextCursor *string       `json:"next_cursor,omitempty" optional:"true"`
}
type Location struct {
	NodeID        string                    `json:"node_id"`
	Available     bool                      `json:"available"`
	ReturnContent bool                      `json:"return_content"`
	Evidence      contract.ResourceEvidence `json:"evidence"`
}
type ResourceSummary struct {
	Resource  string     `json:"resource"`
	Locations []Location `json:"locations"`
}
type ResourcesPage struct {
	Resources  []ResourceSummary `json:"resources"`
	NextCursor *string           `json:"next_cursor,omitempty" optional:"true"`
}
type Candidate struct {
	NodeID     string `json:"node_id"`
	Eligible   bool   `json:"eligible"`
	ReasonCode string `json:"reason_code"`
}
type Placement struct {
	RuleVersion    string      `json:"rule_version"`
	SelectedNodeID string      `json:"selected_node_id"`
	Candidates     []Candidate `json:"candidates"`
}
type ExecutionView struct {
	ExecutionID     string                 `json:"execution_id"`
	RequestID       string                 `json:"request_id"`
	CorrelationID   *string                `json:"correlation_id,omitempty" optional:"true"`
	RequestDigest   string                 `json:"request_digest"`
	State           contract.State         `json:"state"`
	CancelRequested bool                   `json:"cancel_requested"`
	NodeID          *string                `json:"node_id,omitempty" optional:"true"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
	StartedAt       *string                `json:"started_at,omitempty" optional:"true"`
	FinishedAt      *string                `json:"finished_at,omitempty" optional:"true"`
	PollAfterMS     int64                  `json:"poll_after_ms"`
	Approval        *contract.ApprovalInfo `json:"approval,omitempty" optional:"true"`
	Placement       *Placement             `json:"placement,omitempty" optional:"true"`
	Outcome         *contract.Outcome      `json:"outcome,omitempty" optional:"true"`
}
type ArtifactChunk struct {
	ArtifactID    string `json:"artifact_id"`
	Offset        int64  `json:"offset"`
	Encoding      string `json:"encoding"`
	Data          string `json:"data"`
	BytesReturned int64  `json:"bytes_returned"`
	EOF           bool   `json:"eof"`
}

func ValidateView(v ExecutionView) error {
	bad := func() error { return fmt.Errorf("invalid execution view") }
	if !contract.ValidIdentifier(v.ExecutionID) || !contract.ValidIdentifier(v.RequestID) || !contract.ValidDigest(v.RequestDigest) || !v.State.Valid() {
		return bad()
	}
	for _, id := range []*string{v.NodeID, v.CorrelationID} {
		if id != nil && !contract.ValidIdentifier(*id) {
			return bad()
		}
	}
	created, err := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if err != nil {
		return bad()
	}
	updated, err := time.Parse(time.RFC3339Nano, v.UpdatedAt)
	if err != nil || updated.Before(created) {
		return bad()
	}
	var started *time.Time
	for i, raw := range []*string{v.StartedAt, v.FinishedAt} {
		if raw != nil {
			t, err := time.Parse(time.RFC3339Nano, *raw)
			if err != nil || t.Before(created) || t.After(updated) {
				return bad()
			}
			if i == 0 {
				started = &t
			} else if started != nil && t.Before(*started) {
				return bad()
			}
		}
	}
	if v.State.Terminal() != (v.Outcome != nil) || v.State.Terminal() != (v.FinishedAt != nil) {
		return bad()
	}
	if v.State.Terminal() {
		if v.PollAfterMS != 0 || v.Outcome.State != v.State || contract.ValidateOutcome(*v.Outcome) != nil {
			return bad()
		}
		if (v.Outcome.Start == contract.Started) != (v.StartedAt != nil) {
			return bad()
		}
	} else if v.PollAfterMS < 100 || v.PollAfterMS > 30000 {
		return bad()
	}
	if v.State == contract.WaitingApproval && v.Approval == nil || v.State.Terminal() && v.Approval != nil {
		return bad()
	}
	if v.Approval != nil {
		if v.State != contract.WaitingApproval && v.State != contract.Unknown {
			return bad()
		}
		if contract.ValidateEvidence(contract.NodeEvidence{Revision: 1, Phase: contract.AwaitingApproval, Approval: v.Approval}) != nil {
			return bad()
		}
	}
	if v.State == contract.Running && v.StartedAt == nil || v.State.HoldsSlot() && v.NodeID == nil {
		return bad()
	}
	if v.Placement != nil {
		if v.NodeID == nil || v.Placement.SelectedNodeID != *v.NodeID || !contract.ValidIdentifier(v.Placement.RuleVersion) || len(v.Placement.Candidates) > 64 {
			return bad()
		}
		seen := map[string]bool{}
		selected := false
		for _, c := range v.Placement.Candidates {
			if seen[c.NodeID] || !contract.ValidIdentifier(c.NodeID) || !contract.ValidIdentifier(c.ReasonCode) {
				return bad()
			}
			seen[c.NodeID] = true
			if c.NodeID == *v.NodeID && c.Eligible {
				selected = true
			}
		}
		if !selected {
			return bad()
		}
	}
	return nil
}
