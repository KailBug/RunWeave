package contract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"runweave/internal/strictjson"
)

type State string

const (
	Created         State = "CREATED"
	Dispatching     State = "DISPATCHING"
	WaitingApproval State = "WAITING_APPROVAL"
	Running         State = "RUNNING"
	Unknown         State = "UNKNOWN"
	Succeeded       State = "SUCCEEDED"
	Failed          State = "FAILED"
	Rejected        State = "REJECTED"
	StateCancelled  State = "CANCELLED"
	StateTimedOut   State = "TIMED_OUT"
	Expired         State = "EXPIRED"
)

func (s State) Terminal() bool {
	switch s {
	case Succeeded, Failed, Rejected, StateCancelled, StateTimedOut, Expired:
		return true
	}
	return false
}
func (s State) HoldsSlot() bool {
	switch s {
	case Dispatching, WaitingApproval, Running, Unknown:
		return true
	}
	return false
}
func (s State) Valid() bool { return s == Created || s.HoldsSlot() || s.Terminal() }

type StartFact string

const (
	NotStarted StartFact = "not_started"
	Started    StartFact = "started"
	Uncertain  StartFact = "uncertain"
)

type Phase string

const (
	Received         Phase = "RECEIVED"
	AwaitingApproval Phase = "WAITING_APPROVAL"
	StartCommitted   Phase = "START_COMMITTED"
	Executing        Phase = "RUNNING"
	Unresolved       Phase = "UNRESOLVED"
	Finished         Phase = "FINISHED"
	Absent           Phase = "ABSENT"
)

type ApprovalInfo struct {
	ApprovalID     string `json:"approval_id"`
	ExpiresAt      string `json:"expires_at"`
	PolicyVersion  int64  `json:"policy_version"`
	MappingVersion int64  `json:"mapping_version"`
}
type Outcome struct {
	State     State     `json:"state"`
	Start     StartFact `json:"start"`
	Quiescent bool      `json:"quiescent"`
	Result    Result    `json:"result"`
}
type NodeEvidence struct {
	Revision int64         `json:"revision"`
	Phase    Phase         `json:"phase"`
	Approval *ApprovalInfo `json:"approval,omitempty" optional:"true"`
	Outcome  *Outcome      `json:"outcome,omitempty" optional:"true"`
	Start    *StartFact    `json:"start,omitempty" optional:"true"`
}

func ValidIdentifier(value string) bool { return identifier.MatchString(value) }
func ValidDigest(value string) bool {
	return len(value) == 71 && value[:7] == "sha256:" && hexDigest.MatchString(value[7:])
}
func positive(value int64) bool { return value > 0 && value <= MaxSafeInteger }

func ValidateOutcome(outcome Outcome) error {
	if !outcome.State.Terminal() || !outcome.Quiescent || (outcome.Start != NotStarted && outcome.Start != Started) {
		return fmt.Errorf("invalid terminal evidence")
	}
	if _, err := EncodeResult(outcome.Result); err != nil {
		return err
	}
	code := Code("")
	if outcome.Result.Error != nil {
		code = outcome.Result.Error.Code
	}
	switch outcome.State {
	case Succeeded:
		if code != "" || outcome.Start != Started {
			return fmt.Errorf("success requires started and no error")
		}
	case Rejected:
		if outcome.Start != NotStarted {
			return fmt.Errorf("rejection after start")
		}
		switch code {
		case PolicyDenied, ApprovalDenied, ResourceUnavailable, RevisionMismatch, ProgramUnavailable, PathDenied:
		default:
			return fmt.Errorf("invalid rejection code")
		}
	case Expired:
		if code != ApprovalExpired || outcome.Start != NotStarted {
			return fmt.Errorf("invalid expiry")
		}
	case StateCancelled:
		if code != Cancelled {
			return fmt.Errorf("invalid cancellation")
		}
	case StateTimedOut:
		if code != TimedOut || outcome.Start != Started {
			return fmt.Errorf("invalid timeout")
		}
	case Failed:
		switch code {
		case IOError, ProcessStartFailed, ProcessExitNonzero, OutputLimitExceeded, InternalError:
		default:
			return fmt.Errorf("invalid failure code")
		}
		if (code == ProcessStartFailed) != (outcome.Start == NotStarted) {
			return fmt.Errorf("inconsistent start failure")
		}
	}
	if outcome.Start == NotStarted {
		r := outcome.Result
		if r.FSList != nil || r.FSRead != nil || len(r.ArtifactRefs) != 0 {
			return fmt.Errorf("content before start")
		}
		if r.Process != nil && (r.Process.ExitCode != nil || r.Process.StdoutBytes != 0 || r.Process.StderrBytes != 0 || r.Process.StdoutTruncated || r.Process.StderrTruncated) {
			return fmt.Errorf("process output before start")
		}
	}
	return nil
}

func OutcomeDigest(outcome Outcome) (string, error) {
	if err := ValidateOutcome(outcome); err != nil {
		return "", err
	}
	data, err := EncodeResult(outcome.Result)
	if err != nil {
		return "", err
	}
	outcome.Result, err = DecodeResult(data)
	if err != nil {
		return "", err
	}
	return digestValue("runweave-outcome-v1", outcome)
}
func digestValue(version string, value any) (string, error) {
	if !strictjson.ValidStrings(value) {
		return "", fmt.Errorf("invalid UTF-8")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(append([]byte(version+"\n"), data...))
	return fmt.Sprintf("sha256:%x", hash), nil
}

func ValidateEvidence(e NodeEvidence) error {
	if !strictjson.ValidStrings(e) {
		return fmt.Errorf("invalid evidence encoding")
	}
	if e.Phase == Absent {
		if e.Revision != 0 || e.Approval != nil || e.Outcome != nil || e.Start != nil {
			return fmt.Errorf("invalid absent evidence")
		}
		return nil
	}
	if !positive(e.Revision) {
		return fmt.Errorf("invalid journal revision")
	}
	if (e.Phase == Finished) != (e.Outcome != nil) || (e.Phase == AwaitingApproval) != (e.Approval != nil) || (e.Phase == Unresolved) != (e.Start != nil) {
		return fmt.Errorf("evidence payload mismatch")
	}
	switch e.Phase {
	case Received, AwaitingApproval, StartCommitted, Executing, Unresolved, Finished:
	default:
		return fmt.Errorf("unknown journal phase")
	}
	if e.Start != nil && *e.Start != Started && *e.Start != Uncertain {
		return fmt.Errorf("invalid unresolved start evidence")
	}
	if e.Approval != nil {
		a := e.Approval
		if !ValidIdentifier(a.ApprovalID) || !positive(a.PolicyVersion) || !positive(a.MappingVersion) || len(a.ExpiresAt) > 64 {
			return fmt.Errorf("invalid approval binding")
		}
		if _, err := time.Parse(time.RFC3339Nano, a.ExpiresAt); err != nil {
			return fmt.Errorf("invalid approval deadline")
		}
	}
	if e.Outcome != nil {
		return ValidateOutcome(*e.Outcome)
	}
	return nil
}

// ExecutionFact is a storage-independent value. Persist it with the slot/event
// transaction. Evidence must already be bound to the current identity and epoch.
type ExecutionFact struct {
	State           State
	CancelRequested bool
	Start           StartFact
	Revision        int64
	Phase           Phase
	EvidenceDigest  string
	ResultDigest    string
}
type Disposition string

const (
	Applied   Disposition = "applied"
	Duplicate Disposition = "duplicate"
	Stale     Disposition = "stale"
	Conflict  Disposition = "conflict"
)

func NewExecutionFact() ExecutionFact { return ExecutionFact{State: Created, Start: NotStarted} }
func (f ExecutionFact) validate() error {
	if !f.State.Valid() || (f.Start != NotStarted && f.Start != Started && f.Start != Uncertain) || f.Revision < 0 || f.Revision > MaxSafeInteger {
		return fmt.Errorf("invalid execution fact")
	}
	if f.Revision > 0 && (!ValidDigest(f.EvidenceDigest) || phaseRank(f.Phase) == 0) {
		return fmt.Errorf("missing journal identity")
	}
	if f.Revision == 0 && (f.EvidenceDigest != "" || f.Phase != "") {
		return fmt.Errorf("unexpected journal identity")
	}
	if f.State.Terminal() != ValidDigest(f.ResultDigest) || (!f.State.Terminal() && f.ResultDigest != "") {
		return fmt.Errorf("invalid result identity")
	}
	if f.State == Created && (f.Start != NotStarted || f.Revision != 0) || f.State == Running && f.Start != Started {
		return fmt.Errorf("inconsistent execution fact")
	}
	if (f.State == Succeeded || f.State == StateTimedOut) && f.Start != Started || (f.State == Rejected || f.State == Expired || f.State == WaitingApproval) && f.Start != NotStarted {
		return fmt.Errorf("inconsistent state/start")
	}
	if f.State == Dispatching && f.Start == Started || f.Phase == Executing && f.Start != Started || f.Phase == Finished && !f.State.Terminal() {
		return fmt.Errorf("inconsistent phase/start")
	}
	if f.Revision == 0 && (f.State == Running || f.State == WaitingApproval || f.State.Terminal() && f.State != Rejected && f.State != StateCancelled) {
		return fmt.Errorf("state lacks journal evidence")
	}
	return nil
}
func MarkDispatching(f ExecutionFact) (ExecutionFact, error) {
	if err := f.validate(); err != nil {
		return f, err
	}
	if f.State != Created || f.CancelRequested {
		return f, fmt.Errorf("cannot dispatch")
	}
	f.State = Dispatching
	f.Start = Uncertain
	return f, nil
}
func RequestCancellation(f ExecutionFact) (ExecutionFact, error) {
	if err := f.validate(); err != nil {
		return f, err
	}
	if !f.State.Terminal() {
		f.CancelRequested = true
	}
	return f, nil
}
func MarkDisconnected(f ExecutionFact) (ExecutionFact, error) {
	if err := f.validate(); err != nil {
		return f, err
	}
	if f.State.HoldsSlot() {
		f.State = Unknown
		if f.Start != Started {
			f.Start = Uncertain
		}
	}
	return f, nil
}
func FinishLocally(f ExecutionFact, outcome Outcome) (ExecutionFact, error) {
	if err := f.validate(); err != nil {
		return f, err
	}
	if f.State != Created || outcome.Start != NotStarted || (outcome.State != Rejected && outcome.State != StateCancelled) || outcome.State == StateCancelled && !f.CancelRequested {
		return f, fmt.Errorf("cannot finish locally")
	}
	digest, err := OutcomeDigest(outcome)
	if err != nil {
		return f, err
	}
	f.State = outcome.State
	f.ResultDigest = digest
	return f, nil
}
func phaseRank(phase Phase) int {
	switch phase {
	case Received:
		return 1
	case AwaitingApproval:
		return 2
	case StartCommitted:
		return 3
	case Executing, Unresolved:
		return 4
	case Finished:
		return 5
	}
	return 0
}

// reconciled must only be true for a matching response to an outstanding query,
// never for an unsolicited notification. Dispositions do not imply persistence.
func ApplyEvidence(f ExecutionFact, e NodeEvidence, reconciled bool) (ExecutionFact, Disposition, error) {
	if err := f.validate(); err != nil {
		return f, "", err
	}
	if err := ValidateEvidence(e); err != nil {
		return f, "", err
	}
	if f.State == Created {
		return f, "", fmt.Errorf("node evidence before dispatch intent")
	}
	if e.Phase == Absent {
		if !reconciled {
			return f, "", fmt.Errorf("absence requires query response")
		}
		if f.State.Terminal() {
			return f, Stale, nil
		}
		next, err := MarkDisconnected(f)
		return next, Applied, err
	}
	if e.Revision < f.Revision {
		return f, Stale, nil
	}
	// Normalize result defaults before hashing the entire evidence.
	if e.Outcome != nil {
		copyOutcome := *e.Outcome
		raw, _ := EncodeResult(copyOutcome.Result)
		copyOutcome.Result, _ = DecodeResult(raw)
		e.Outcome = &copyOutcome
	}
	digest, err := digestValue("runweave-evidence-v1", e)
	if err != nil {
		return f, "", err
	}
	if e.Revision == f.Revision && digest != f.EvidenceDigest {
		return f, Conflict, nil
	}
	if f.State.Terminal() {
		if e.Outcome != nil {
			resultDigest, _ := OutcomeDigest(*e.Outcome)
			if resultDigest == f.ResultDigest {
				return f, Duplicate, nil
			}
			return f, Conflict, nil
		}
		return f, Stale, nil
	}
	if e.Revision == f.Revision && (!reconciled || f.State != Unknown) {
		return f, Duplicate, nil
	}
	if phaseRank(e.Phase) < phaseRank(f.Phase) {
		return f, Conflict, nil
	}
	next := f
	next.Revision = e.Revision
	next.Phase = e.Phase
	next.EvidenceDigest = digest
	switch e.Phase {
	case Received:
		next.State = Dispatching
		next.Start = NotStarted
	case AwaitingApproval:
		next.State = WaitingApproval
		next.Start = NotStarted
	case StartCommitted:
		next.State = Unknown
		next.Start = Uncertain
	case Executing:
		next.State = Running
		next.Start = Started
	case Unresolved:
		if f.Start == Started && *e.Start != Started {
			return f, Conflict, nil
		}
		next.State = Unknown
		next.Start = *e.Start
	case Finished:
		o := e.Outcome
		if f.Start == Started && o.Start != Started {
			return f, Conflict, nil
		}
		if (f.Phase == StartCommitted || f.Phase == Executing || f.Phase == Unresolved) && o.Start == NotStarted && (o.Result.Error == nil || o.Result.Error.Code != ProcessStartFailed) {
			return f, Conflict, nil
		}
		next.State = o.State
		next.Start = o.Start
		next.ResultDigest, _ = OutcomeDigest(*o)
	}
	return next, Applied, nil
}
