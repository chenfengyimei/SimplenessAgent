package contracts

import "fmt"

type ErrorCode string

const (
	ErrInvalidInput          ErrorCode = "INVALID_INPUT"
	ErrNotFound              ErrorCode = "NOT_FOUND"
	ErrInvalidTransition     ErrorCode = "INVALID_STATE_TRANSITION"
	ErrPlanInvalid           ErrorCode = "PLAN_INVALID"
	ErrPathDenied            ErrorCode = "PATH_OUTSIDE_WORKSPACE"
	ErrPermissionDenied      ErrorCode = "PERMISSION_DENIED"
	ErrApprovalRequired      ErrorCode = "APPROVAL_REQUIRED"
	ErrSideEffectUnknown     ErrorCode = "SIDE_EFFECT_UNKNOWN"
	ErrStorage               ErrorCode = "STORAGE_ERROR"
	ErrArtifactCorrupt       ErrorCode = "ARTIFACT_CORRUPT"
	ErrAuthenticationFailed  ErrorCode = "AUTHENTICATION_FAILED"
	ErrEndpointUnreachable   ErrorCode = "ENDPOINT_UNREACHABLE"
	ErrRateLimited           ErrorCode = "RATE_LIMITED"
	ErrRequestTimeout        ErrorCode = "REQUEST_TIMEOUT"
	ErrRequestCancelled      ErrorCode = "REQUEST_CANCELLED"
	ErrContextOverflow       ErrorCode = "CONTEXT_OVERFLOW"
	ErrOutputLimitReached    ErrorCode = "OUTPUT_LIMIT_REACHED"
	ErrInvalidResponse       ErrorCode = "INVALID_RESPONSE"
	ErrInvalidToolCall       ErrorCode = "INVALID_TOOL_CALL"
	ErrContentRejected       ErrorCode = "CONTENT_REJECTED"
	ErrModelUnavailable      ErrorCode = "MODEL_UNAVAILABLE"
	ErrCapabilityUnsupported ErrorCode = "CAPABILITY_UNSUPPORTED"
	ErrProviderInternal      ErrorCode = "PROVIDER_INTERNAL_ERROR"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code ErrorCode, message string) error {
	return &Error{Code: code, Message: message}
}
