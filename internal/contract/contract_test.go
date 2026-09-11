package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "../../docs/资源/契约"

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(data)
}

func requireCode(t testing.TB, err error, want Code) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func TestFixtures(t *testing.T) {
	var cases []struct {
		File string
		Kind string
		Code Code
	}
	if err := json.Unmarshal(fixture(t, "cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if seen[tc.File] {
			t.Fatalf("duplicate fixture %s", tc.File)
		}
		seen[tc.File] = true
		t.Run(tc.File, func(t *testing.T) {
			data := fixture(t, tc.File)
			switch tc.Kind {
			case "request":
				r, err := DecodeRequest(data)
				requireCode(t, err, tc.Code)
				if err == nil {
					if _, err := RequestDigest(r); err != nil {
						t.Fatal(err)
					}
				} else if r.RequestID != "" {
					t.Fatal("partial request escaped on error")
				}
			case "result":
				r, err := DecodeResult(data)
				requireCode(t, err, tc.Code)
				if err == nil {
					encoded, err := EncodeResult(r)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := DecodeResult(encoded); err != nil {
						t.Fatal(err)
					}
				} else if r.Operation != "" {
					t.Fatal("partial result escaped on error")
				}
			default:
				t.Fatalf("unknown fixture kind %s", tc.Kind)
			}
		})
	}
	files, err := filepath.Glob(filepath.Join(fixtureDir, "*-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		if strings.HasPrefix(name, "request-") || strings.HasPrefix(name, "result-") {
			if !seen[name] {
				t.Fatalf("fixture not exercised: %s", name)
			}
		}
	}
}

func TestCanonicalVector(t *testing.T) {
	r, err := DecodeRequest(fixture(t, "request-list.json"))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, fixture(t, "list-canonical.json")) {
		t.Fatalf("unexpected canonical JSON: %s", canonical)
	}
	digest, err := RequestDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	if digest != string(fixture(t, "list-digest.txt")) {
		t.Fatalf("unexpected digest: %s", digest)
	}
}

func TestDigestEquivalenceAndSensitivity(t *testing.T) {
	base := string(fixture(t, "request-list.json"))
	digest := func(input string) string {
		t.Helper()
		r, err := DecodeRequest([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		d, err := RequestDigest(r)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	want := digest(base)
	for name, input := range map[string]string{
		"request-id":          strings.Replace(base, "list-001", "different-key", 1),
		"defaults-order-sets": `{"timeout_ms":120000,"operation":{"max_entries":1000,"target":{"relative_path":".","resource":"repo://demo"},"type":"fs.list"},"requirements":{"resources":[{"resource":"repo://demo"},{"resource":"repo://demo"}],"capabilities":["fs.list","fs.list"],"os":"linux"},"request_id":"list-001"}`,
		"escaped-value":       strings.Replace(base, "repo://demo", `repo://d\u0065mo`, -1),
		"whitespace":          " \n\t" + base + " \r\n",
	} {
		t.Run(name, func(t *testing.T) {
			if digest(input) != want {
				t.Fatal("equivalent request changed digest")
			}
		})
	}
	for name, input := range map[string]string{
		"timeout":     strings.Replace(base, `"operation":`, `"timeout_ms":1,"operation":`, 1),
		"correlation": strings.Replace(base, `"operation":`, `"correlation_id":"group-1","operation":`, 1),
		"path":        strings.Replace(base, `"relative_path":"."`, `"relative_path":"src"`, 1),
		"placement":   strings.Replace(base, `"resources":`, `"node_id":"node-a","resources":`, 1),
		"capability":  strings.Replace(base, `"resources":`, `"capabilities":["gpu"],"resources":`, 1),
		"revision":    strings.Replace(base, `{"resource":"repo://demo"}`, `{"resource":"repo://demo","revision":{"type":"git_commit","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if digest(input) == want {
				t.Fatal("semantic change kept digest")
			}
		})
	}
	process := string(fixture(t, "request-process.json"))
	before := digest(process)
	for _, input := range []string{
		strings.Replace(process, `{"literal":"scripts/analyze.py"},{"literal":"--input"}`, `{"literal":"--input"},{"literal":"scripts/analyze.py"}`, 1),
		strings.Replace(process, `"--input"`, `" --input "`, 1),
	} {
		if digest(input) == before {
			t.Fatal("argv semantics lost")
		}
	}
	r, err := DecodeRequest([]byte(process))
	if err != nil {
		t.Fatal(err)
	}
	if literal := *(*r.Operation.Args)[4].Literal; literal != "$(echo literal) repo://not-a-reference" {
		t.Fatalf("literal changed: %q", literal)
	}
	r.Requirements.Capabilities = []string{"z", "a"}
	saved, _ := json.Marshal(r)
	if _, err := RequestDigest(r); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(saved, after) {
		t.Fatal("digest mutated caller")
	}
}

func TestStrictJSONAndRequestRejections(t *testing.T) {
	base := string(fixture(t, "request-list.json"))
	insert := func(s string) string { return strings.Replace(base, `"operation":`, s+`,"operation":`, 1) }
	cases := map[string]string{
		"duplicate":              insert(`"request_id":"second"`),
		"escaped-duplicate":      insert(`"request_\u0069d":"second"`),
		"case-alias":             insert(`"Request_ID":"second"`),
		"nested-case":            strings.Replace(base, `"target":`, `"Target":`, 1),
		"unknown-nested":         strings.Replace(base, `"relative_path":`, `"root":"/tmp","relative_path":`, 1),
		"duplicate-nested":       strings.Replace(base, `"relative_path":`, `"relative_path":"x","relative_path":`, 1),
		"null":                   insert(`"timeout_ms":null`),
		"missing":                strings.Replace(base, `"request_id":"list-001",`, "", 1),
		"empty-correlation":      insert(`"correlation_id":""`),
		"exponent":               insert(`"timeout_ms":1e3`),
		"fraction":               insert(`"timeout_ms":1000.0`),
		"negative":               insert(`"timeout_ms":-1`),
		"zero":                   insert(`"timeout_ms":0`),
		"overflow":               insert(`"timeout_ms":9223372036854775808`),
		"too-long":               insert(`"timeout_ms":3600001`),
		"trailing":               base + ` {}`,
		"root-array":             `[]`,
		"root-null":              `null`,
		"surrogate-high":         strings.Replace(base, `"."`, `"\ud800"`, 1),
		"surrogate-low":          strings.Replace(base, `"."`, `"\udc00"`, 1),
		"surrogate-pair-invalid": strings.Replace(base, `"."`, `"\ud800\u0041"`, 1),
		"invalid-utf8":           strings.Replace(base, `"."`, "\"\xff\"", 1),
		"mixed-operation":        strings.Replace(base, `"target":`, `"args":[],"target":`, 1),
		"unknown-os":             strings.Replace(base, `"resources":`, `"os":"windows","resources":`, 1),
		"conflicting-resource":   strings.Replace(base, `[{"resource":"repo://demo"}]`, `[{"resource":"repo://demo"},{"resource":"repo://demo","revision":{"type":"git_commit","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]`, 1),
		"depth":                  insert(`"extension":` + strings.Repeat("[", 33) + `1` + strings.Repeat("]", 33)),
	}
	for _, path := range []string{"", "/etc/passwd", "../secret", "a/../b", "./a", "a//b", "a/", "a\\b", "C:/secret", "a\nfile"} {
		encoded, _ := json.Marshal(path)
		cases["path-"+path] = strings.Replace(base, `"relative_path":"."`, `"relative_path":`+string(encoded), 1)
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) { _, err := DecodeRequest([]byte(input)); requireCode(t, err, InvalidRequest) })
	}
	for _, value := range []string{`"\ud83d\ude00"`, `"中文"`, `"a%2fb"`} {
		// % is a literal path character, never URL-decoded into a separator.
		_, err := DecodeRequest([]byte(strings.Replace(base, `"relative_path":"."`, `"relative_path":`+value, 1)))
		requireCode(t, err, "")
	}
}

func TestRequestOperationLimits(t *testing.T) {
	load := func(name string) Request {
		t.Helper()
		r, err := DecodeRequest(fixture(t, name))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	cases := []struct {
		name, file string
		change     func(*Request)
		code       Code
	}{
		{"id-limit", "request-list.json", func(r *Request) { r.RequestID = strings.Repeat("x", 129) }, InvalidRequest},
		{"entries-limit", "request-list.json", func(r *Request) { r.Operation.MaxEntries = ptr(1001) }, InvalidRequest},
		{"empty-resources", "request-list.json", func(r *Request) { r.Requirements.Resources = []ResourceRequirement{} }, InvalidRequest},
		{"resource-uri", "request-list.json", func(r *Request) { r.Requirements.Resources[0].Resource = "repo://demo?x=1" }, InvalidRequest},
		{"revision", "request-process.json", func(r *Request) { r.Requirements.Resources[1].Revision = &Revision{"git_commit", "abc"} }, InvalidRequest},
		{"limit-zero", "request-read.json", func(r *Request) { r.Operation.Limit = ptr(int64(0)) }, InvalidRequest},
		{"limit-max", "request-read.json", func(r *Request) { r.Operation.Limit = ptr(int64(65536)) }, ""},
		{"limit-over", "request-read.json", func(r *Request) { r.Operation.Limit = ptr(int64(65537)) }, InvalidRequest},
		{"offset-overflow", "request-read.json", func(r *Request) { r.Operation.Offset = ptr(MaxSafeInteger) }, InvalidRequest},
		{"program-path", "request-process.json", func(r *Request) { r.Operation.Program = ptr("/bin/sh") }, InvalidRequest},
		{"args-both", "request-process.json", func(r *Request) { (*r.Operation.Args)[0].Path = r.Operation.Cwd }, InvalidRequest},
		{"args-neither", "request-process.json", func(r *Request) { (*r.Operation.Args)[0] = Argument{} }, InvalidRequest},
		{"args-nul", "request-process.json", func(r *Request) { (*r.Operation.Args)[0].Literal = ptr("a\x00b") }, InvalidRequest},
		{"args-bytes", "request-process.json", func(r *Request) { (*r.Operation.Args)[0].Literal = ptr(strings.Repeat("中", 1366)) }, InvalidRequest},
		{"args-reference", "request-process.json", func(r *Request) { (*r.Operation.Args)[2].Path.Resource = "dataset://missing" }, UndeclaredResource},
		{"cwd-reference", "request-process.json", func(r *Request) { r.Operation.Cwd.Resource = "repo://missing" }, UndeclaredResource},
		{"args-count", "request-process.json", func(r *Request) {
			args := make([]Argument, 257)
			for i := range args {
				args[i].Literal = ptr("")
			}
			r.Operation.Args = &args
		}, InvalidRequest},
		{"args-total", "request-process.json", func(r *Request) {
			args := make([]Argument, 9)
			for i := range args {
				args[i].Literal = ptr(strings.Repeat("x", 4096))
			}
			r.Operation.Args = &args
		}, InvalidRequest},
		{"path-bytes", "request-list.json", func(r *Request) { r.Operation.Target.RelativePath = strings.Repeat("中", 1366) }, InvalidRequest},
		{"caps-derived-limit", "request-list.json", func(r *Request) {
			r.Requirements.Capabilities = nil
			for i := 0; i < 32; i++ {
				r.Requirements.Capabilities = append(r.Requirements.Capabilities, fmt.Sprintf("cap-%d", i))
			}
		}, InvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := load(tc.file)
			tc.change(&r)
			_, err := RequestDigest(r)
			requireCode(t, err, tc.code)
		})
	}
	base := fixture(t, "request-list.json")
	boundary := append(bytes.Clone(base), bytes.Repeat([]byte(" "), MaxRequestBytes-len(base))...)
	_, err := DecodeRequest(boundary)
	requireCode(t, err, "")
	_, err = DecodeRequest(append(boundary, ' '))
	requireCode(t, err, RequestTooLarge)
	r := load("request-process.json")
	(*r.Operation.Args)[0].Literal = ptr(string([]byte{0xff}))
	_, err = RequestDigest(r)
	requireCode(t, err, InvalidRequest)
}

func TestResultValidation(t *testing.T) {
	cases := []struct {
		name, file string
		change     func(*Result)
		valid      bool
	}{
		{"default-unknown", "result-list.json", func(r *Result) { r.Mutations = nil }, true},
		{"false-coverage", "result-list.json", func(r *Result) { r.Mutations = &Mutations{ptr("complete")} }, false},
		{"negative-time", "result-list.json", func(r *Result) { r.DurationMS = -1 }, false},
		{"unsafe-integer", "result-list.json", func(r *Result) { r.DurationMS = MaxSafeInteger + 1 }, false},
		{"missing-body", "result-list.json", func(r *Result) { r.FSList = nil }, false},
		{"wrong-body", "result-list.json", func(r *Result) { r.Process = &ProcessResult{} }, false},
		{"duplicate-entry", "result-list.json", func(r *Result) { r.FSList.Entries = append(r.FSList.Entries, r.FSList.Entries[0]) }, false},
		{"entry-path", "result-list.json", func(r *Result) { r.FSList.Entries[0].Name = "../secret" }, false},
		{"entry-kind", "result-list.json", func(r *Result) { r.FSList.Entries[0].Kind = "device" }, false},
		{"entry-size", "result-list.json", func(r *Result) { r.FSList.Entries[0].SizeBytes = ptr(int64(-1)) }, false},
		{"duplicate-evidence", "result-list.json", func(r *Result) { r.Resources = append(r.Resources, r.Resources[0]) }, false},
		{"unknown-with-revision", "result-list.json", func(r *Result) { r.Resources[0].Revision = &Revision{"git_commit", strings.Repeat("a", 40)} }, false},
		{"git-without-clean", "result-process.json", func(r *Result) { r.Resources[0].Clean = nil }, false},
		{"claimed-digest", "result-read.json", func(r *Result) { r.Resources[0].Source = "content_digest" }, false},
		{"artifact-path", "result-process.json", func(r *Result) { r.ArtifactRefs[0].ArtifactID = "/var/log/out" }, false},
		{"artifact-type", "result-process.json", func(r *Result) { r.ArtifactRefs[0].MediaType = "text" }, false},
		{"artifact-expiry", "result-process.json", func(r *Result) { r.ArtifactRefs[0].ExpiresAt = "2026-09-17" }, false},
		{"artifact-duplicate", "result-process.json", func(r *Result) { r.ArtifactRefs = append(r.ArtifactRefs, r.ArtifactRefs[0]) }, false},
		{"base64-newline", "result-read.json", func(r *Result) { r.FSRead.Data = "AP8K\n" }, false},
		{"base64-pad-bits", "result-read.json", func(r *Result) { r.FSRead.Data = "AB=="; r.FSRead.BytesReturned = 1 }, false},
		{"byte-count", "result-read.json", func(r *Result) { r.FSRead.BytesReturned = 1 }, false},
		{"utf8-count", "result-read.json", func(r *Result) { r.FSRead.Encoding = "utf8"; r.FSRead.Data = "中文"; r.FSRead.BytesReturned = 6 }, true},
		{"read-overflow", "result-read.json", func(r *Result) { r.FSRead.Offset = MaxSafeInteger }, false},
		{"success-no-exit", "result-process.json", func(r *Result) { r.Process.ExitCode = nil }, false},
		{"exit-range", "result-process.json", func(r *Result) { r.Process.ExitCode = ptr(int64(256)) }, false},
		{"nonzero-without-body", "result-failed.json", func(r *Result) { r.Process = nil }, false},
		{"nonzero-zero-exit", "result-failed.json", func(r *Result) { r.Process.ExitCode = ptr(int64(0)) }, false},
		{"unknown-error", "result-denied.json", func(r *Result) { r.Error.Code = "UNKNOWN" }, false},
		{"call-error", "result-denied.json", func(r *Result) { r.Error.Code = StorageUnavailable }, false},
		{"long-error", "result-denied.json", func(r *Result) { r.Error.Message = strings.Repeat("中", 342) }, false},
		{"invalid-utf8", "result-read.json", func(r *Result) { r.FSRead.Data = string([]byte{0xff}) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := DecodeResult(fixture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&r)
			data, err := EncodeResult(r)
			if tc.valid {
				requireCode(t, err, "")
				decoded, err := DecodeResult(data)
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Mutations == nil {
					t.Fatal("missing default mutations")
				}
			} else {
				requireCode(t, err, InvalidResult)
			}
		})
	}
	base := string(fixture(t, "result-list.json"))
	for name, input := range map[string]string{
		"unknown":         strings.Replace(base, `"duration_ms":`, `"stdout":"secret","duration_ms":`, 1),
		"null-array":      strings.Replace(base, `"artifact_refs":[]`, `"artifact_refs":null`, 1),
		"missing-boolean": strings.Replace(base, `,"truncated":false`, "", 1),
		"missing-count":   strings.Replace(base, `"duration_ms":2,`, "", 1),
		"duplicate":       strings.Replace(base, `"duration_ms":`, `"duration_ms":0,"duration_ms":`, 1),
	} {
		t.Run(name, func(t *testing.T) { _, err := DecodeResult([]byte(input)); requireCode(t, err, InvalidResult) })
	}
}

func TestResultContentAndEnvelopeLimits(t *testing.T) {
	r, err := DecodeResult(fixture(t, "result-read.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.FSRead.Encoding = "utf8"
	r.FSRead.Data = strings.Repeat("x", MaxContentBytes)
	r.FSRead.BytesReturned = MaxContentBytes
	_, err = EncodeResult(r)
	requireCode(t, err, "")
	r.FSRead.Data += "x"
	r.FSRead.BytesReturned++
	_, err = EncodeResult(r)
	requireCode(t, err, InvalidResult)
	list, err := DecodeResult(fixture(t, "result-list.json"))
	if err != nil {
		t.Fatal(err)
	}
	list.FSList.Entries = []Entry{}
	remaining := MaxContentBytes
	for i := 0; remaining > 0; i++ {
		count := min(255, remaining)
		name := strings.Repeat("z", count)
		if count >= 4 {
			name = fmt.Sprintf("%04d", i) + strings.Repeat("x", count-4)
		}
		list.FSList.Entries = append(list.FSList.Entries, Entry{Name: name, Kind: "file"})
		remaining -= count
	}
	_, err = EncodeResult(list)
	requireCode(t, err, "")
	list.FSList.Entries = append(list.FSList.Entries, Entry{Name: "z", Kind: "file"})
	_, err = EncodeResult(list)
	requireCode(t, err, InvalidResult)
	base := fixture(t, "result-list.json")
	boundary := append(bytes.Clone(base), bytes.Repeat([]byte(" "), MaxResultBytes-len(base))...)
	_, err = DecodeResult(boundary)
	requireCode(t, err, "")
	_, err = DecodeResult(append(boundary, ' '))
	requireCode(t, err, InvalidResult)
}

func TestErrorClasses(t *testing.T) {
	for code, want := range map[Code]ErrorClass{InvalidRequest: CallError, StorageUnavailable: CallError, IdempotencyConflict: CallError, ArtifactUnavailable: LookupError, ProcessExitNonzero: ExecutionError, Cancelled: ExecutionError, InvalidResult: ValidationError, "UNKNOWN": "", "": ""} {
		if code.Class() != want {
			t.Fatalf("%s: %s", code, code.Class())
		}
	}
}

func FuzzRequestRoundTrip(f *testing.F) {
	for _, name := range []string{"request-list.json", "request-read.json", "request-process.json", "request-invalid-path.json"} {
		f.Add(fixture(f, name))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := DecodeRequest(data)
		if err != nil {
			return
		}
		digest, err := RequestDigest(r)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		again, err := DecodeRequest(wire)
		if err != nil {
			t.Fatal(err)
		}
		other, err := RequestDigest(again)
		if err != nil || other != digest {
			t.Fatalf("unstable digest: %v", err)
		}
	})
}

func FuzzResultRoundTrip(f *testing.F) {
	for _, name := range []string{"result-list.json", "result-read.json", "result-process.json", "result-denied.json"} {
		f.Add(fixture(f, name))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := DecodeResult(data)
		if err != nil {
			return
		}
		wire, err := EncodeResult(r)
		if err != nil {
			t.Fatal(err)
		}
		again, err := DecodeResult(wire)
		if err != nil {
			t.Fatal(err)
		}
		second, err := EncodeResult(again)
		if err != nil || !bytes.Equal(wire, second) {
			t.Fatalf("unstable result: %v", err)
		}
	})
}
