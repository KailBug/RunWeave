package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"runweave/internal/contract"
)

type sample struct {
	Name    string
	Sender  Sender
	Epoch   string
	Valid   bool
	Message json.RawMessage
}

func samples(t testing.TB) []sample {
	t.Helper()
	data, err := os.ReadFile("../../docs/资源/Node协议/消息样例.json")
	if err != nil {
		t.Fatal(err)
	}
	var out []sample
	if json.Unmarshal(data, &out) != nil {
		t.Fatal("invalid fixture JSON")
	}
	return out
}
func example(t testing.TB, name string) (Envelope, Context) {
	t.Helper()
	for _, s := range samples(t) {
		if s.Name == name {
			var e Envelope
			if err := json.Unmarshal(s.Message, &e); err != nil {
				t.Fatal(err)
			}
			return e, Context{s.Sender, "node-a", s.Epoch}
		}
	}
	t.Fatal("missing sample")
	return Envelope{}, Context{}
}
func payload(t testing.TB, e *Envelope, p any) {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	e.Payload = raw
}

func TestProtocolFixtures(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range samples(t) {
		t.Run(s.Name, func(t *testing.T) {
			ctx := Context{s.Sender, "node-a", s.Epoch}
			m, err := Decode(s.Message, ctx)
			if (err == nil) != s.Valid {
				t.Fatalf("valid=%v: %v", s.Valid, err)
			}
			if err == nil {
				seen[m.Envelope.Type] = true
				encoded, err := Encode(m.Envelope, ctx)
				if err != nil {
					t.Fatal(err)
				}
				again, err := Decode(encoded, ctx)
				if err != nil {
					t.Fatal(err)
				}
				second, _ := Encode(again.Envelope, ctx)
				if !bytes.Equal(encoded, second) {
					t.Fatal("unstable wire encoding")
				}
			}
		})
	}
	if len(seen) != 14 {
		t.Fatalf("only %d message types covered", len(seen))
	}
}

func TestProtocolSessionAndPayloadRejections(t *testing.T) {
	for name, change := range map[string]func(*Envelope, *Context){
		"old-epoch":              func(e *Envelope, c *Context) { c.Epoch = "epoch-new" },
		"unknown-version":        func(e *Envelope, c *Context) { e.ProtocolVersion = 2 },
		"wrong-sender":           func(e *Envelope, c *Context) { c.Sender = Server },
		"unknown-sender":         func(e *Envelope, c *Context) { c.Sender = "" },
		"no-session":             func(e *Envelope, c *Context) { c.Epoch = "" },
		"execution-on-heartbeat": func(e *Envelope, c *Context) { v := "e1"; e.ExecutionID = &v },
		"invalid-id":             func(e *Envelope, c *Context) { e.MessageID = "../x" },
		"unknown-field":          func(e *Envelope, c *Context) { e.Payload = json.RawMessage(`{"config_version":1,"online":true}`) },
		"duplicate":              func(e *Envelope, c *Context) { e.Payload = json.RawMessage(`{"config_version":1,"config_version":2}`) },
		"case":                   func(e *Envelope, c *Context) { e.Payload = json.RawMessage(`{"Config_version":1}`) },
		"zero-version":           func(e *Envelope, c *Context) { e.Payload = json.RawMessage(`{"config_version":0}`) },
		"null":                   func(e *Envelope, c *Context) { e.Payload = json.RawMessage(`{"config_version":null}`) },
		"surrogate": func(e *Envelope, c *Context) {
			e.Extensions = map[string]json.RawMessage{"x": json.RawMessage(`"\ud800"`)}
		},
		"extension-overflow": func(e *Envelope, c *Context) {
			e.Extensions = map[string]json.RawMessage{"x": json.RawMessage(`"` + strings.Repeat("<", 1000) + `"`)}
		},
		"invalid-extension-key": func(e *Envelope, c *Context) {
			e.Extensions = map[string]json.RawMessage{string([]byte{0xff}): json.RawMessage(`1`)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, c := example(t, "heartbeat")
			change(&e, &c)
			if _, err := Encode(e, c); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	for _, name := range []string{"register", "registered"} {
		e, c := example(t, name)
		c.NodeID = "node-b"
		if _, err := Encode(e, c); err == nil {
			t.Fatal("forged node identity accepted")
		}
	}
	e, c := example(t, "dispatch")
	var d Dispatch
	json.Unmarshal(e.Payload, &d)
	d.RequestDigest = "sha256:" + strings.Repeat("a", 64)
	payload(t, &e, d)
	if _, err := Encode(e, c); err == nil {
		t.Fatal("wrong request digest accepted")
	}
	e, c = example(t, "accepted")
	var o Observation
	json.Unmarshal(e.Payload, &o)
	o.Evidence.Phase = contract.Executing
	payload(t, &e, o)
	if _, err := Encode(e, c); err == nil {
		t.Fatal("wrong evidence phase accepted")
	}
	e, c = example(t, "artifact-response")
	var a ArtifactResponse
	json.Unmarshal(e.Payload, &a)
	a.Error = &contract.Error{Code: contract.Forbidden, Message: "denied"}
	payload(t, &e, a)
	if _, err := Encode(e, c); err == nil {
		t.Fatal("mixed artifact success/error")
	}
	a.Data = nil
	a.BytesReturned = nil
	a.EOF = nil
	payload(t, &e, a)
	if _, err := Encode(e, c); err != nil {
		t.Fatal(err)
	}
}

func TestReplyBinding(t *testing.T) {
	query, _ := example(t, "query")
	snapshot, _ := example(t, "snapshot")
	if err := MatchReply(query, snapshot, "node-a"); err != nil {
		t.Fatal(err)
	}
	var p Snapshot
	json.Unmarshal(snapshot.Payload, &p)
	p.QueryMessageID = "unrelated"
	payload(t, &snapshot, p)
	if MatchReply(query, snapshot, "node-a") == nil {
		t.Fatal("unrelated query matched")
	}
	p.QueryMessageID = query.MessageID
	p.Evidence = contract.NodeEvidence{Revision: 6, Phase: contract.Unresolved, Start: new(contract.Started)}
	payload(t, &snapshot, p)
	if err := MatchReply(query, snapshot, "node-a"); err != nil {
		t.Fatal(err)
	}
	result, _ := example(t, "result")
	receipt, _ := example(t, "receipt")
	if MatchReply(result, receipt, "node-a") == nil {
		t.Fatal("wrong outcome digest matched")
	}
	var obs Observation
	json.Unmarshal(result.Payload, &obs)
	digest, err := contract.OutcomeDigest(*obs.Evidence.Outcome)
	if err != nil {
		t.Fatal(err)
	}
	payload(t, &receipt, Receipt{obs.RequestDigest, digest})
	if err := MatchReply(result, receipt, "node-a"); err != nil {
		t.Fatal(err)
	}
	receipt.ConnectionEpoch = "epoch-other"
	if MatchReply(result, receipt, "node-a") == nil {
		t.Fatal("old-session receipt matched")
	}
	request, _ := example(t, "artifact-request")
	response, _ := example(t, "artifact-response")
	if err := MatchReply(request, response, "node-a"); err != nil {
		t.Fatal(err)
	}
	var ar ArtifactRequest
	json.Unmarshal(request.Payload, &ar)
	ar.Limit = 1
	payload(t, &request, ar)
	if MatchReply(request, response, "node-a") == nil {
		t.Fatal("oversized artifact reply matched")
	}
}

func TestFrameAndExtensionLimits(t *testing.T) {
	e, c := example(t, "heartbeat")
	e.Extensions = map[string]json.RawMessage{"trace": json.RawMessage(`{"optional":"可忽略"}`)}
	if _, err := Encode(e, c); err != nil {
		t.Fatal(err)
	}
	e.Extensions = map[string]json.RawMessage{"x": json.RawMessage(`"` + strings.Repeat("x", 4094) + `"`)}
	if _, err := Encode(e, c); err != nil {
		t.Fatal(err)
	}
	e.Extensions["x"] = json.RawMessage(`"` + strings.Repeat("x", 4095) + `"`)
	if _, err := Encode(e, c); err == nil {
		t.Fatal("extension >4096 accepted")
	}
	e, c = example(t, "heartbeat")
	raw, err := Encode(e, c)
	if err != nil {
		t.Fatal(err)
	}
	boundary := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), MaxFrameBytes-len(raw))...)
	if _, err := Decode(boundary, c); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(append(boundary, ' '), c); err == nil {
		t.Fatal("oversized frame accepted")
	}
}

func FuzzProtocolRoundTrip(f *testing.F) {
	for _, s := range samples(f) {
		f.Add([]byte(s.Message), string(s.Sender), s.Epoch)
	}
	f.Fuzz(func(t *testing.T, data []byte, sender, epoch string) {
		ctx := Context{Sender(sender), "node-a", epoch}
		m, err := Decode(data, ctx)
		if err != nil {
			return
		}
		wire, err := Encode(m.Envelope, ctx)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(wire, ctx)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Encode(again.Envelope, ctx)
		if err != nil || !bytes.Equal(wire, second) {
			t.Fatalf("unstable roundtrip: %v", err)
		}
	})
}
