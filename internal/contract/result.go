package contract

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"reflect"
	"strings"
	"time"
)

type Result struct {
	Operation    string             `json:"operation"`
	DurationMS   int64              `json:"duration_ms"`
	Resources    []ResourceEvidence `json:"resources"`
	Mutations    *Mutations         `json:"mutations,omitempty" optional:"true"`
	ArtifactRefs []ArtifactRef      `json:"artifact_refs"`
	Error        *Error             `json:"error,omitempty" optional:"true"`
	FSList       *ListResult        `json:"fs_list,omitempty" optional:"true"`
	FSRead       *ReadResult        `json:"fs_read,omitempty" optional:"true"`
	Process      *ProcessResult     `json:"process,omitempty" optional:"true"`
}

type Mutations struct {
	Coverage *string `json:"coverage,omitempty" optional:"true"`
}

type ResourceEvidence struct {
	Resource string    `json:"resource"`
	Source   string    `json:"source"`
	Revision *Revision `json:"revision,omitempty" optional:"true"`
	Clean    *bool     `json:"clean,omitempty" optional:"true"`
}

type ArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	MediaType  string `json:"media_type"`
	SizeBytes  int64  `json:"size_bytes"`
	ExpiresAt  string `json:"expires_at"`
	Truncated  bool   `json:"truncated"`
}

type ListResult struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

type Entry struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	SizeBytes *int64 `json:"size_bytes,omitempty" optional:"true"`
}

type ReadResult struct {
	Offset        int64  `json:"offset"`
	Encoding      string `json:"encoding"`
	Data          string `json:"data"`
	BytesReturned int64  `json:"bytes_returned"`
	EOF           bool   `json:"eof"`
}

type ProcessResult struct {
	ExitCode        *int64 `json:"exit_code,omitempty" optional:"true"`
	StdoutBytes     int64  `json:"stdout_bytes"`
	StderrBytes     int64  `json:"stderr_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}

func DecodeResult(data []byte) (Result, error) {
	var result Result
	if len(data) > MaxResultBytes {
		return Result{}, fail(InvalidResult, "result exceeds 1 MiB")
	}
	if err := strictDecode(data, &result); err != nil {
		return Result{}, fail(InvalidResult, "invalid result JSON structure, fields or encoding")
	}
	if err := result.validate(); err != nil {
		return Result{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxResultBytes {
		return Result{}, fail(InvalidResult, "normalized result exceeds 1 MiB")
	}
	return result, nil
}

func EncodeResult(result Result) ([]byte, error) {
	if !validGoStrings(reflect.ValueOf(result)) {
		return nil, fail(InvalidResult, "invalid UTF-8 in result")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fail(InvalidResult, "result cannot be encoded")
	}
	validated, err := DecodeResult(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(validated)
}

func nonnegative(value int64) bool { return value >= 0 && value <= MaxSafeInteger }

func (r *Result) validate() error {
	bad := func() error { return fail(InvalidResult, "invalid result fields or limits") }
	if !nonnegative(r.DurationMS) || len(r.Resources) > 64 || len(r.ArtifactRefs) > 16 {
		return bad()
	}
	if r.Mutations == nil {
		r.Mutations = &Mutations{}
	}
	if r.Mutations.Coverage == nil {
		r.Mutations.Coverage = ptr("unknown")
	}
	if *r.Mutations.Coverage != "unknown" && *r.Mutations.Coverage != "best_effort" {
		return bad()
	}
	if r.Error != nil {
		if r.Error.Code.Class() != ExecutionError || len(r.Error.Message) == 0 || len(r.Error.Message) > 1024 || strings.ContainsRune(r.Error.Message, 0) {
			return bad()
		}
		if (r.Error.Code == ProcessExitNonzero || r.Error.Code == ProcessStartFailed) && r.Operation != ProcessExec {
			return bad()
		}
	}
	seen := map[string]bool{}
	for _, evidence := range r.Resources {
		if seen[evidence.Resource] || !validResource(evidence.Resource) || !validRevision(evidence.Resource, evidence.Revision) {
			return bad()
		}
		seen[evidence.Resource] = true
		switch evidence.Source {
		case "unknown":
			if evidence.Revision != nil || evidence.Clean != nil {
				return bad()
			}
		case "git":
			if evidence.Revision == nil || evidence.Revision.Type != "git_commit" || evidence.Clean == nil {
				return bad()
			}
		case "owner_manifest":
			if evidence.Revision == nil || evidence.Revision.Type != "owner_revision" || evidence.Clean != nil {
				return bad()
			}
		case "content_digest":
			if evidence.Revision == nil || evidence.Revision.Type != "sha256" || evidence.Clean != nil {
				return bad()
			}
		default:
			return bad()
		}
	}
	seen = map[string]bool{}
	for _, artifact := range r.ArtifactRefs {
		if seen[artifact.ArtifactID] || !identifier.MatchString(artifact.ArtifactID) || !nonnegative(artifact.SizeBytes) || len(artifact.MediaType) > 128 || len(artifact.ExpiresAt) > 64 {
			return bad()
		}
		seen[artifact.ArtifactID] = true
		media, _, err := mime.ParseMediaType(artifact.MediaType)
		if err != nil || !strings.Contains(media, "/") {
			return bad()
		}
		if _, err := time.Parse(time.RFC3339Nano, artifact.ExpiresAt); err != nil {
			return bad()
		}
	}
	switch r.Operation {
	case FSList:
		if r.FSRead != nil || r.Process != nil || r.Error == nil && r.FSList == nil {
			return bad()
		}
		if r.FSList != nil {
			if len(r.FSList.Entries) > 1000 {
				return bad()
			}
			seen = map[string]bool{}
			total := 0
			for _, entry := range r.FSList.Entries {
				if seen[entry.Name] || len(entry.Name) > 255 || entry.Name == "." || !validPath(entry.Name) || strings.Contains(entry.Name, "/") || entry.SizeBytes != nil && !nonnegative(*entry.SizeBytes) {
					return bad()
				}
				seen[entry.Name] = true
				switch entry.Kind {
				case "file", "directory", "symlink", "other":
				default:
					return bad()
				}
				total += len(entry.Name)
			}
			if total > MaxContentBytes {
				return bad()
			}
		}
	case FSRead:
		if r.FSList != nil || r.Process != nil || r.Error == nil && r.FSRead == nil {
			return bad()
		}
		if r.FSRead != nil {
			v := r.FSRead
			if !nonnegative(v.Offset) || v.BytesReturned < 0 || v.BytesReturned > MaxContentBytes || v.Offset > MaxSafeInteger-v.BytesReturned {
				return bad()
			}
			var count int
			switch v.Encoding {
			case "utf8":
				count = len(v.Data)
			case "base64":
				if len(v.Data) > base64.StdEncoding.EncodedLen(MaxContentBytes) {
					return bad()
				}
				decoded, err := base64.StdEncoding.Strict().DecodeString(v.Data)
				if err != nil || base64.StdEncoding.EncodeToString(decoded) != v.Data {
					return bad()
				}
				count = len(decoded)
			default:
				return bad()
			}
			if int64(count) != v.BytesReturned {
				return bad()
			}
		}
	case ProcessExec:
		if r.FSList != nil || r.FSRead != nil || r.Error == nil && r.Process == nil {
			return bad()
		}
		if r.Process != nil {
			v := r.Process
			if !nonnegative(v.StdoutBytes) || !nonnegative(v.StderrBytes) || v.ExitCode != nil && (*v.ExitCode < 0 || *v.ExitCode > 255) {
				return bad()
			}
			if r.Error == nil && (v.ExitCode == nil || *v.ExitCode != 0) {
				return bad()
			}
		}
		if r.Error != nil && r.Error.Code == ProcessExitNonzero && (r.Process == nil || r.Process.ExitCode == nil || *r.Process.ExitCode == 0) {
			return bad()
		}
	default:
		return bad()
	}
	return nil
}
