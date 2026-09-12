package contract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"runweave/internal/strictjson"
)

const (
	DigestVersion          = "runweave-request-v1"
	MaxRequestBytes        = 64 << 10
	MaxResultBytes         = 1 << 20
	MaxContentBytes        = 64 << 10
	MaxSafeInteger   int64 = 9007199254740991
	DefaultTimeoutMS int64 = 120000
	MaxTimeoutMS     int64 = 3600000
	FSList                 = "fs.list"
	FSRead                 = "fs.read"
	ProcessExec            = "process.exec"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var hexDigest = regexp.MustCompile(`^[0-9a-f]+$`)

type Request struct {
	RequestID     string       `json:"request_id"`
	CorrelationID *string      `json:"correlation_id,omitempty" optional:"true"`
	Requirements  Requirements `json:"requirements"`
	Operation     Operation    `json:"operation"`
	TimeoutMS     *int64       `json:"timeout_ms,omitempty" optional:"true"`
}

type Requirements struct {
	OS            *string               `json:"os,omitempty" optional:"true"`
	Capabilities  []string              `json:"capabilities" optional:"true"`
	Resources     []ResourceRequirement `json:"resources"`
	NodeID        *string               `json:"node_id,omitempty" optional:"true"`
	PreferredNode *string               `json:"preferred_node,omitempty" optional:"true"`
}

type Revision struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type ResourceRequirement struct {
	Resource string    `json:"resource"`
	Revision *Revision `json:"revision,omitempty" optional:"true"`
}

type ResourcePath struct {
	Resource     string `json:"resource"`
	RelativePath string `json:"relative_path"`
}

type Argument struct {
	Literal *string       `json:"literal,omitempty" optional:"true"`
	Path    *ResourcePath `json:"path,omitempty" optional:"true"`
}

type Operation struct {
	Type       string        `json:"type"`
	Target     *ResourcePath `json:"target,omitempty" optional:"true"`
	MaxEntries *int          `json:"max_entries,omitempty" optional:"true"`
	Offset     *int64        `json:"offset,omitempty" optional:"true"`
	Limit      *int64        `json:"limit,omitempty" optional:"true"`
	Program    *string       `json:"program,omitempty" optional:"true"`
	Cwd        *ResourcePath `json:"cwd,omitempty" optional:"true"`
	Args       *[]Argument   `json:"args,omitempty" optional:"true"`
}

func ptr[T any](v T) *T { return &v }

// DecodeRequest validates before returning a normalized independent value.
// On failure no execution ID exists in this package and no side effect occurs.
func DecodeRequest(data []byte) (Request, error) {
	var request Request
	if len(data) > MaxRequestBytes {
		return Request{}, fail(RequestTooLarge, "request exceeds 65536 bytes")
	}
	if err := strictjson.Decode(data, &request); err != nil {
		return Request{}, fail(InvalidRequest, "invalid request JSON structure, fields or encoding")
	}
	if err := request.normalize(); err != nil {
		return Request{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > MaxRequestBytes {
		return Request{}, fail(RequestTooLarge, "normalized request exceeds 65536 bytes")
	}
	return request, nil
}

// CanonicalRequest revalidates even programmatically constructed values, and
// never mutates the caller's slices/pointers. Use its bytes only with DigestVersion.
func CanonicalRequest(request Request) ([]byte, error) {
	if !strictjson.ValidStrings(request) {
		return nil, fail(InvalidRequest, "invalid UTF-8 in request")
	}
	// Optional capabilities normalize to []; nil must not marshal to wire null.
	if request.Requirements.Capabilities == nil {
		request.Requirements.Capabilities = []string{}
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fail(InvalidRequest, "request cannot be encoded")
	}
	r, err := DecodeRequest(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		CorrelationID *string      `json:"correlation_id,omitempty"`
		Requirements  Requirements `json:"requirements"`
		Operation     Operation    `json:"operation"`
		TimeoutMS     int64        `json:"timeout_ms"`
	}{r.CorrelationID, r.Requirements, r.Operation, *r.TimeoutMS})
}

func RequestDigest(request Request) (string, error) {
	canonical, err := CanonicalRequest(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(DigestVersion+"\n"), canonical...))
	return fmt.Sprintf("sha256:%x", sum), nil
}

func (r *Request) normalize() error {
	bad := func(message string) error { return fail(InvalidRequest, message) }
	if !identifier.MatchString(r.RequestID) || !optionalID(r.CorrelationID) {
		return bad("invalid request/correlation identifier")
	}
	if r.TimeoutMS == nil {
		r.TimeoutMS = ptr(DefaultTimeoutMS)
	}
	if *r.TimeoutMS < 1 || *r.TimeoutMS > MaxTimeoutMS {
		return bad("timeout_ms out of range")
	}
	q := &r.Requirements
	if q.OS == nil {
		q.OS = ptr("linux")
	}
	if *q.OS != "linux" || !optionalID(q.NodeID) || !optionalID(q.PreferredNode) {
		return bad("invalid placement requirements")
	}
	if len(q.Capabilities) > 32 || len(q.Resources) < 1 || len(q.Resources) > 64 {
		return bad("requirements count out of range")
	}
	for _, capability := range q.Capabilities {
		if !identifier.MatchString(capability) {
			return bad("invalid capability")
		}
	}
	declared := map[string]*Revision{}
	for _, resource := range q.Resources {
		if !validResource(resource.Resource) || !validRevision(resource.Resource, resource.Revision) {
			return bad("invalid resource or revision")
		}
		if previous, exists := declared[resource.Resource]; exists && !sameRevision(previous, resource.Revision) {
			return bad("conflicting resource requirements")
		}
		declared[resource.Resource] = resource.Revision
	}
	refs, err := r.Operation.normalize()
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if _, ok := declared[ref.Resource]; !ok {
			return fail(UndeclaredResource, "operation references an undeclared resource")
		}
	}
	q.Resources = q.Resources[:0]
	for uri, revision := range declared {
		q.Resources = append(q.Resources, ResourceRequirement{uri, revision})
	}
	slices.SortFunc(q.Resources, func(a, b ResourceRequirement) int { return strings.Compare(a.Resource, b.Resource) })
	q.Capabilities = append(q.Capabilities, r.Operation.Type)
	slices.Sort(q.Capabilities)
	q.Capabilities = slices.Compact(q.Capabilities)
	if len(q.Capabilities) > 32 {
		return bad("normalized capabilities exceed 32")
	}
	return nil
}

func (o *Operation) normalize() ([]ResourcePath, error) {
	bad := func() ([]ResourcePath, error) { return nil, fail(InvalidRequest, "invalid operation fields or limits") }
	var refs []ResourcePath
	switch o.Type {
	case FSList, FSRead:
		if o.Target == nil || o.Program != nil || o.Cwd != nil || o.Args != nil {
			return bad()
		}
		refs = append(refs, *o.Target)
		if o.Type == FSList {
			if o.Offset != nil || o.Limit != nil {
				return bad()
			}
			if o.MaxEntries == nil {
				o.MaxEntries = ptr(1000)
			}
			if *o.MaxEntries < 1 || *o.MaxEntries > 1000 {
				return bad()
			}
		} else {
			if o.MaxEntries != nil {
				return bad()
			}
			if o.Offset == nil {
				o.Offset = ptr(int64(0))
			}
			if o.Limit == nil {
				o.Limit = ptr(int64(MaxContentBytes))
			}
			if *o.Offset < 0 || *o.Limit < 1 || *o.Limit > MaxContentBytes || *o.Offset > MaxSafeInteger-*o.Limit {
				return bad()
			}
		}
	case ProcessExec:
		if o.Target != nil || o.MaxEntries != nil || o.Offset != nil || o.Limit != nil || o.Program == nil || !identifier.MatchString(*o.Program) || o.Cwd == nil {
			return bad()
		}
		refs = append(refs, *o.Cwd)
		if o.Args == nil {
			o.Args = ptr([]Argument{})
		}
		if len(*o.Args) > 256 {
			return bad()
		}
		total := 0
		for _, arg := range *o.Args {
			if (arg.Literal == nil) == (arg.Path == nil) {
				return bad()
			}
			if arg.Path != nil {
				refs = append(refs, *arg.Path)
				continue
			}
			if len(*arg.Literal) > 4096 || strings.ContainsRune(*arg.Literal, 0) {
				return bad()
			}
			total += len(*arg.Literal)
		}
		if total > 32768 {
			return bad()
		}
	default:
		return nil, fail(UnsupportedOperation, "unsupported operation")
	}
	for _, ref := range refs {
		if !validResource(ref.Resource) || !validPath(ref.RelativePath) {
			return bad()
		}
	}
	return refs, nil
}

func optionalID(value *string) bool { return value == nil || identifier.MatchString(*value) }
func validResource(uri string) bool {
	for _, prefix := range []string{"repo://", "dataset://"} {
		if strings.HasPrefix(uri, prefix) {
			return identifier.MatchString(strings.TrimPrefix(uri, prefix))
		}
	}
	return false
}
func validPath(path string) bool {
	if path == "." {
		return true
	}
	if len(path) == 0 || len(path) > 4096 || strings.ContainsAny(path, "\\:") {
		return false
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return false
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func validRevision(resource string, revision *Revision) bool {
	if revision == nil {
		return true
	}
	if strings.HasPrefix(resource, "repo://") {
		return revision.Type == "git_commit" && (len(revision.Value) == 40 || len(revision.Value) == 64) && hexDigest.MatchString(revision.Value)
	}
	switch revision.Type {
	case "owner_revision":
		return identifier.MatchString(revision.Value)
	case "sha256":
		return len(revision.Value) == 64 && hexDigest.MatchString(revision.Value)
	default:
		return false
	}
}
func sameRevision(a, b *Revision) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
