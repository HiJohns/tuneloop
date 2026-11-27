package services

import (
	"errors"
	"net/http"
)

// IAMFailureKind classifies an upstream IAM failure (#2598) so that handlers can
// answer with a consistent triple — HTTP status ≡ business code family ≡
// actionable user message:
//
//	transient  → 502 / 50200 → «请稍后再试»   (network / timeout / upstream 429 / 5xx)
//	conflict   → 409 / 40900 → the conflicting object + how to resolve it
//	userAction → 400 / 40002 → the user must redo the action (upstream 400 / 422)
//	config     → 500 / 50015 → «联系平台管理员» (upstream 401 / 403 / 404 / other 4xx)
//
// The point of the classification is that a *persistent* failure is never
// advertised as retryable and a *transient* failure is never advertised as a
// data conflict (#2597 scan, #2598 follow-up).
type IAMFailureKind int

const (
	// IAMFailureTransient: retrying later may succeed (transport error, 429, 5xx).
	IAMFailureTransient IAMFailureKind = iota
	// IAMFailureConflict: the request conflicts with existing data (upstream 409).
	IAMFailureConflict
	// IAMFailureUserAction: the upstream rejected the request payload (400 / 422);
	// the caller's user must redo the action (re-scan, re-issue).
	IAMFailureUserAction
	// IAMFailureConfig: persistent configuration / permission / data problem
	// (401 / 403 / 404 / other 4xx). Retrying is pointless; an operator must fix
	// the namespace, the permission template or the referenced data.
	IAMFailureConfig
)

// String keeps log output stable and greppable (`kind=config`).
func (k IAMFailureKind) String() string {
	switch k {
	case IAMFailureTransient:
		return "transient"
	case IAMFailureConflict:
		return "conflict"
	case IAMFailureUserAction:
		return "user_action"
	case IAMFailureConfig:
		return "config"
	default:
		return "unknown"
	}
}

// ClassifyIAMFailure returns the failure kind and the upstream HTTP status
// (0 = no upstream status, i.e. a transport-level failure such as a dial error
// or a context deadline).
//
// Only errors.As is used — never string matching. Both typed upstream errors are
// supported:
//   - *IAMAPIError      (wx-bind / wx-merge / wx-merge-confirm paths, iam.go)
//   - *IAMClientHTTPError (IAMClient paths such as CreateOrganization, iam_client.go)
//
// Wrapping the typed error with fmt.Errorf("…: %w") is transparent.
// A nil error is classified as transient status 0; callers must not pass nil
// (it is a programming error), the branch only exists so the helper never panics
// and never answers 200.
func ClassifyIAMFailure(err error) (IAMFailureKind, int) {
	if err == nil {
		return IAMFailureTransient, 0
	}
	var apiErr *IAMAPIError
	if errors.As(err, &apiErr) {
		return classifyIAMUpstreamStatus(apiErr.StatusCode), apiErr.StatusCode
	}
	var httpErr *IAMClientHTTPError
	if errors.As(err, &httpErr) {
		return classifyIAMUpstreamStatus(httpErr.Status), httpErr.Status
	}
	return IAMFailureTransient, 0
}

// classifyIAMUpstreamStatus is the single status→kind predicate.
func classifyIAMUpstreamStatus(status int) IAMFailureKind {
	switch {
	case status == 0:
		// No upstream status: transport failure (dial/timeout/TLS).
		return IAMFailureTransient
	case status == http.StatusConflict:
		return IAMFailureConflict
	case status == http.StatusBadRequest, status == http.StatusUnprocessableEntity:
		return IAMFailureUserAction
	case status == http.StatusTooManyRequests, status >= 500:
		return IAMFailureTransient
	default:
		// 401 / 403 / 404 / any other 4xx (and unexpected 3xx):
		// persistent — an operator has to fix configuration or permissions.
		return IAMFailureConfig
	}
}
