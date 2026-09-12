package contract

import (
	"encoding/json"
	"testing"
)

func successOutcome() Outcome {
	return Outcome{State: Succeeded, Start: Started, Quiescent: true, Result: Result{Operation: FSList, Resources: []ResourceEvidence{}, ArtifactRefs: []ArtifactRef{}, FSList: &ListResult{Entries: []Entry{}}}}
}
func errorOutcome(state State, start StartFact, code Code) Outcome {
	return Outcome{State: state, Start: start, Quiescent: true, Result: Result{Operation: ProcessExec, Resources: []ResourceEvidence{}, ArtifactRefs: []ArtifactRef{}, Error: &Error{code, "测试错误"}}}
}
func dispatchFact(t testing.TB) ExecutionFact {
	t.Helper()
	f, err := MarkDispatching(NewExecutionFact())
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func advance(t testing.TB, f ExecutionFact, e NodeEvidence, reconcile bool, want Disposition) ExecutionFact {
	t.Helper()
	next, got, err := ApplyEvidence(f, e, reconcile)
	if err != nil || got != want {
		t.Fatalf("%s/%s: disposition %s want %s: %v", f.State, e.Phase, got, want, err)
	}
	return next
}
func TestLifecycleEvidenceAndCancelRace(t *testing.T) {
	f := dispatchFact(t)
	if !f.State.HoldsSlot() {
		t.Fatal("dispatch did not occupy slot")
	}
	e := NodeEvidence{Revision: 1, Phase: Received}
	f = advance(t, f, e, false, Applied)
	wait := NodeEvidence{Revision: 2, Phase: AwaitingApproval, Approval: &ApprovalInfo{"a1", "2026-09-11T10:10:00Z", 1, 1}}
	f = advance(t, f, wait, false, Applied)
	if f.State != WaitingApproval || !f.State.HoldsSlot() {
		t.Fatal("approval did not hold slot")
	}
	f, _ = MarkDisconnected(f)
	f, _ = RequestCancellation(f)
	duplicate := advance(t, f, wait, false, Duplicate)
	if duplicate != f || f.State != Unknown || !f.State.HoldsSlot() || !f.CancelRequested {
		t.Fatal("disconnect/cancel lost facts")
	}
	f = advance(t, f, wait, true, Applied)
	if f.State != WaitingApproval || !f.CancelRequested {
		t.Fatal("snapshot did not restore approval")
	}
	f = advance(t, f, NodeEvidence{Revision: 3, Phase: StartCommitted}, false, Applied)
	if f.State != Unknown || !f.State.HoldsSlot() {
		t.Fatal("ambiguous start did not stay unknown")
	}
	f = advance(t, f, NodeEvidence{Revision: 4, Phase: Executing}, false, Applied)
	if f.State != Running || f.Start != Started {
		t.Fatal("start evidence missing")
	}
	if late := advance(t, f, e, false, Stale); late != f {
		t.Fatal("late accepted regressed state")
	}
	o := successOutcome()
	finished := NodeEvidence{Revision: 5, Phase: Finished, Outcome: &o}
	f = advance(t, f, finished, false, Applied)
	if f.State != Succeeded || !f.CancelRequested || f.State.HoldsSlot() {
		t.Fatal("completion/cancel race corrupted outcome")
	}
	if duplicate := advance(t, f, finished, false, Duplicate); duplicate != f {
		t.Fatal("duplicate result changed terminal")
	}
	changed := o
	changed.Result.DurationMS = 10
	if next := advance(t, f, NodeEvidence{Revision: 5, Phase: Finished, Outcome: &changed}, false, Conflict); next != f {
		t.Fatal("conflict mutated terminal")
	}
	if next := advance(t, f, NodeEvidence{Revision: 6, Phase: Finished, Outcome: &changed}, false, Conflict); next != f {
		t.Fatal("new revision overwrote terminal")
	}
	if next, _ := RequestCancellation(f); next != f {
		t.Fatal("late cancel changed terminal")
	}
}

func TestLifecycleMissingAndContradictoryEvidence(t *testing.T) {
	t.Run("skipped-notifications", func(t *testing.T) {
		o := successOutcome()
		f := advance(t, dispatchFact(t), NodeEvidence{Revision: 5, Phase: Finished, Outcome: &o}, false, Applied)
		if f.State != Succeeded {
			t.Fatal(f)
		}
	})
	t.Run("absent-is-not-cancelled", func(t *testing.T) {
		f := dispatchFact(t)
		f, _ = RequestCancellation(f)
		f = advance(t, f, NodeEvidence{Phase: Absent}, true, Applied)
		if f.State != Unknown || !f.State.HoldsSlot() || !f.CancelRequested {
			t.Fatal(f)
		}
		if _, _, err := ApplyEvidence(f, NodeEvidence{Phase: Absent}, false); err == nil {
			t.Fatal("unsolicited absence accepted")
		}
		if _, err := MarkDispatching(f); err == nil {
			t.Fatal("unknown redispatched")
		}
	})
	t.Run("same-revision-conflict", func(t *testing.T) {
		f := advance(t, dispatchFact(t), NodeEvidence{Revision: 1, Phase: Received}, false, Applied)
		if next := advance(t, f, NodeEvidence{Revision: 1, Phase: Executing}, false, Conflict); next != f {
			t.Fatal("conflict changed fact")
		}
	})
	t.Run("regression-and-start-contradiction", func(t *testing.T) {
		f := advance(t, dispatchFact(t), NodeEvidence{Revision: 4, Phase: Executing}, false, Applied)
		f, _ = MarkDisconnected(f)
		if next := advance(t, f, NodeEvidence{Revision: 5, Phase: Received}, true, Conflict); next != f {
			t.Fatal("journal regressed")
		}
		o := errorOutcome(Rejected, NotStarted, PolicyDenied)
		if next := advance(t, f, NodeEvidence{Revision: 6, Phase: Finished, Outcome: &o}, true, Conflict); next != f {
			t.Fatal("start fact regressed")
		}
	})
	t.Run("committed-window", func(t *testing.T) {
		f := advance(t, dispatchFact(t), NodeEvidence{Revision: 3, Phase: StartCommitted}, true, Applied)
		o := errorOutcome(StateCancelled, NotStarted, Cancelled)
		advance(t, f, NodeEvidence{Revision: 4, Phase: Finished, Outcome: &o}, true, Conflict)
		o = errorOutcome(Failed, NotStarted, ProcessStartFailed)
		f = advance(t, f, NodeEvidence{Revision: 4, Phase: Finished, Outcome: &o}, true, Applied)
		if f.State != Failed || f.State.HoldsSlot() {
			t.Fatal(f)
		}
	})
	t.Run("local-cancel", func(t *testing.T) {
		f := NewExecutionFact()
		o := errorOutcome(StateCancelled, NotStarted, Cancelled)
		if _, err := FinishLocally(f, o); err == nil {
			t.Fatal("cancel without intent")
		}
		f, _ = RequestCancellation(f)
		if _, err := MarkDispatching(f); err == nil {
			t.Fatal("dispatch after cancel")
		}
		f, err := FinishLocally(f, o)
		if err != nil || f.State != StateCancelled {
			t.Fatalf("%+v %v", f, err)
		}
		if _, err := FinishLocally(dispatchFact(t), o); err == nil {
			t.Fatal("locally cancelled possible dispatch")
		}
		if _, _, err := ApplyEvidence(NewExecutionFact(), NodeEvidence{Revision: 1, Phase: Received}, false); err == nil {
			t.Fatal("evidence before dispatch accepted")
		}
	})
}

func TestOutcomeEvidenceRules(t *testing.T) {
	valid := []Outcome{successOutcome(), errorOutcome(Rejected, NotStarted, PolicyDenied), errorOutcome(Expired, NotStarted, ApprovalExpired), errorOutcome(StateCancelled, NotStarted, Cancelled), errorOutcome(StateCancelled, Started, Cancelled), errorOutcome(StateTimedOut, Started, TimedOut), errorOutcome(Failed, NotStarted, ProcessStartFailed), errorOutcome(Failed, Started, IOError)}
	for _, o := range valid {
		if err := ValidateOutcome(o); err != nil {
			t.Fatalf("%s: %v", o.State, err)
		}
		if _, err := OutcomeDigest(o); err != nil {
			t.Fatal(err)
		}
	}
	for name, modify := range map[string]func(*Outcome){
		"cleanup-unknown":     func(o *Outcome) { o.Quiescent = false },
		"uncertain-start":     func(o *Outcome) { o.Start = Uncertain },
		"success-not-started": func(o *Outcome) { o.Start = NotStarted },
		"nonterminal":         func(o *Outcome) { o.State = Running },
		"wrong-error":         func(o *Outcome) { o.Result.Error = &Error{Cancelled, "cancel"} },
	} {
		t.Run(name, func(t *testing.T) {
			o := successOutcome()
			modify(&o)
			if ValidateOutcome(o) == nil {
				t.Fatal("invalid terminal accepted")
			}
		})
	}
	before := successOutcome()
	after := before
	after.Result.Mutations = &Mutations{ptr("unknown")}
	a, _ := OutcomeDigest(before)
	b, _ := OutcomeDigest(after)
	if a != b {
		t.Fatal("default changed result digest")
	}
	notStarted := errorOutcome(Rejected, NotStarted, PolicyDenied)
	notStarted.Result.Process = &ProcessResult{StdoutBytes: 1}
	if ValidateOutcome(notStarted) == nil {
		t.Fatal("output before start")
	}
	for _, e := range []NodeEvidence{{Revision: 0, Phase: Received}, {Revision: 1, Phase: Absent}, {Revision: 1, Phase: AwaitingApproval}, {Revision: 1, Phase: Finished}, {Revision: 1, Phase: "NEW"}, {Revision: 1, Phase: Received, Approval: &ApprovalInfo{}}, {Revision: 1, Phase: AwaitingApproval, Approval: &ApprovalInfo{"a", "bad-time", 1, 1}}} {
		if ValidateEvidence(e) == nil {
			t.Fatalf("invalid evidence accepted: %+v", e)
		}
	}
}

func TestUnresolvedRestartEvidence(t *testing.T) {
	f := advance(t, dispatchFact(t), NodeEvidence{Revision: 4, Phase: Executing}, false, Applied)
	f, _ = MarkDisconnected(f)
	e := NodeEvidence{Revision: 5, Phase: Unresolved, Start: ptr(Started)}
	f = advance(t, f, e, true, Applied)
	if f.State != Unknown || f.Start != Started || !f.State.HoldsSlot() {
		t.Fatal("restart released or erased known start")
	}
	if next := advance(t, f, NodeEvidence{Revision: 6, Phase: Unresolved, Start: ptr(Uncertain)}, true, Conflict); next != f {
		t.Fatal("known start erased")
	}
	if next := advance(t, f, NodeEvidence{Revision: 6, Phase: Received}, true, Conflict); next != f {
		t.Fatal("restart regressed to unstarted")
	}
	f = advance(t, f, NodeEvidence{Revision: 6, Phase: Executing}, true, Applied)
	o := successOutcome()
	f = advance(t, f, NodeEvidence{Revision: 7, Phase: Finished, Outcome: &o}, true, Applied)
	if f.State != Succeeded {
		t.Fatal(f)
	}
	if ValidateEvidence(NodeEvidence{Revision: 1, Phase: Unresolved}) == nil {
		t.Fatal("missing start on unresolved")
	}
	if ValidateEvidence(NodeEvidence{Revision: 1, Phase: Received, Start: ptr(Started)}) == nil {
		t.Fatal("unexpected start field")
	}
}

func FuzzLifecycleEvidence(f *testing.F) {
	seed, _ := json.Marshal(NodeEvidence{Revision: 1, Phase: Received})
	f.Add(seed)
	o := successOutcome()
	seed, _ = json.Marshal(NodeEvidence{Revision: 5, Phase: Finished, Outcome: &o})
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		var e NodeEvidence
		if json.Unmarshal(data, &e) != nil {
			return
		}
		before := dispatchFact(t)
		next, _, err := ApplyEvidence(before, e, true)
		if err != nil {
			if next != before {
				t.Fatal("failure mutated fact")
			}
			return
		}
		if err := next.validate(); err != nil {
			t.Fatal(err)
		}
		if next.State.Terminal() && next.State.HoldsSlot() {
			t.Fatal("terminal holds slot")
		}
	})
}
