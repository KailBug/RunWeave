package contract

type ErrorClass string

const (
	CallError       ErrorClass = "call"
	LookupError     ErrorClass = "lookup"
	ExecutionError  ErrorClass = "execution"
	ValidationError ErrorClass = "validation"
)

type Code string

const (
	InvalidRequest       Code = "INVALID_REQUEST"
	RequestTooLarge      Code = "REQUEST_TOO_LARGE"
	UnsupportedOperation Code = "UNSUPPORTED_OPERATION"
	UndeclaredResource   Code = "UNDECLARED_RESOURCE"
	Unauthenticated      Code = "UNAUTHENTICATED"
	Forbidden            Code = "FORBIDDEN"
	IdempotencyConflict  Code = "IDEMPOTENCY_CONFLICT"
	NoMatchingNode       Code = "NO_MATCHING_NODE"
	NodeBusy             Code = "NODE_BUSY"
	StorageUnavailable   Code = "STORAGE_UNAVAILABLE"
	ExecutionNotFound    Code = "EXECUTION_NOT_FOUND"
	ArtifactNotFound     Code = "ARTIFACT_NOT_FOUND"
	ArtifactUnavailable  Code = "ARTIFACT_UNAVAILABLE"
	ArtifactExpired      Code = "ARTIFACT_EXPIRED"
	PolicyDenied         Code = "POLICY_DENIED"
	ApprovalDenied       Code = "APPROVAL_DENIED"
	ApprovalExpired      Code = "APPROVAL_EXPIRED"
	ResourceUnavailable  Code = "RESOURCE_UNAVAILABLE"
	RevisionMismatch     Code = "REVISION_MISMATCH"
	ProgramUnavailable   Code = "PROGRAM_UNAVAILABLE"
	PathDenied           Code = "PATH_DENIED"
	IOError              Code = "IO_ERROR"
	ProcessStartFailed   Code = "PROCESS_START_FAILED"
	ProcessExitNonzero   Code = "PROCESS_EXIT_NONZERO"
	Cancelled            Code = "CANCELLED"
	TimedOut             Code = "TIMED_OUT"
	OutputLimitExceeded  Code = "OUTPUT_LIMIT_EXCEEDED"
	InternalError        Code = "INTERNAL_ERROR"
	InvalidResult        Code = "INVALID_RESULT"
)

// Class describes where an error belongs, not whether retrying can create work.
// In particular, STORAGE_UNAVAILABLE can mean an uncertain commit outcome.
func (c Code) Class() ErrorClass {
	switch c {
	case InvalidRequest, RequestTooLarge, UnsupportedOperation, UndeclaredResource,
		Unauthenticated, Forbidden, IdempotencyConflict, NoMatchingNode, NodeBusy, StorageUnavailable:
		return CallError
	case ExecutionNotFound, ArtifactNotFound, ArtifactUnavailable, ArtifactExpired:
		return LookupError
	case PolicyDenied, ApprovalDenied, ApprovalExpired, ResourceUnavailable, RevisionMismatch,
		ProgramUnavailable, PathDenied, IOError, ProcessStartFailed, ProcessExitNonzero,
		Cancelled, TimedOut, OutputLimitExceeded, InternalError:
		return ExecutionError
	case InvalidResult:
		return ValidationError
	default:
		return ""
	}
}

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string             { return string(e.Code) + ": " + e.Message }
func fail(code Code, message string) error { return &Error{Code: code, Message: message} }
