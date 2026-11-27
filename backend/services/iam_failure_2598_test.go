package services

// #2598 R1：IAM 上游失败分类器（services.ClassifyIAMFailure）。
//
// 断言三件事：
//  1. 每种上游状态落到正确的类别（transient / conflict / userAction / config），
//     且 `IAMAPIError`（wx-* 路径）与 `IAMClientHTTPError`（IAMClient 路径）判定一致；
//  2. `%w` 包装链不破坏 `errors.As`（包装后退化为 transient 就是漏判）；
//  3. `IAMClientHTTPError.Error()` 文本与改造前**逐字一致**（历史日志/断言口径不变）。
//
// Token lines are printed with fmt.Printf so scripts/verify-2598.sh can grep them
// regardless of go test -v indentation.

import (
	"fmt"
	"testing"
)

func TestClassifyIAMFailure2598(t *testing.T) {
	want := map[int]IAMFailureKind{
		0:   IAMFailureTransient,  // 无上游状态（传输层/超时）
		400: IAMFailureUserAction, // 请求被上游校验拒绝 → 用户重做
		401: IAMFailureConfig,     // 持久配置/权限
		403: IAMFailureConfig,
		404: IAMFailureConfig,
		409: IAMFailureConflict,
		422: IAMFailureUserAction,
		429: IAMFailureTransient, // 限流 → 可重试
		500: IAMFailureTransient,
		503: IAMFailureTransient,
	}

	// 13 个场景 = 10 个上游状态（每个同时覆盖两种错误类型）+ 包装链 + 传输层 + nil。
	type scene struct {
		name  string
		errs  []error
		kind  IAMFailureKind
		state int
	}
	scenes := make([]scene, 0, 13)
	statuses := []int{0, 400, 401, 403, 404, 409, 422, 429, 500, 503}
	for _, status := range statuses {
		scenes = append(scenes, scene{
			name: fmt.Sprintf("status-%d", status),
			errs: []error{
				&IAMAPIError{StatusCode: status, ErrorCode: "upstream", Body: `{"error":"upstream"}`},
				&IAMClientHTTPError{Op: "CreateOrganization", Status: status, Body: `{"error":"upstream"}`},
			},
			kind:  want[status],
			state: status,
		})
	}
	scenes = append(scenes,
		scene{
			name:  "wrapped-403",
			errs:  []error{fmt.Errorf("CreateAdminTransitSite: %w", &IAMAPIError{StatusCode: 403, Body: `{"message":"denied"}`})},
			kind:  IAMFailureConfig,
			state: 403,
		},
		scene{
			name:  "transport",
			errs:  []error{fmt.Errorf("failed to call IAM wx-bind endpoint: %w", errStub2598("dial tcp 127.0.0.1:5561: connect: connection refused"))},
			kind:  IAMFailureTransient,
			state: 0,
		},
		scene{
			name:  "nil",
			errs:  []error{nil},
			kind:  IAMFailureTransient,
			state: 0,
		},
	)

	mismatch := 0
	for _, sc := range scenes {
		for _, err := range sc.errs {
			kind, status := ClassifyIAMFailure(err)
			if kind != sc.kind || status != sc.state {
				mismatch++
				t.Errorf("%s: kind=%s status=%d, want kind=%s status=%d (err=%v)",
					sc.name, kind, status, sc.kind, sc.state, err)
			}
		}
	}

	// Error() 文本逐字保持（旧串：403 = "permission denied: <body>"，其余 = "<Op> returned status <n>: <body>"）。
	textKept := 1
	if got, expected := (&IAMClientHTTPError{
		Op: "CreateOrganization", Status: 403, Body: `{"message":"permission denied"}`,
	}).Error(), `permission denied: {"message":"permission denied"}`; got != expected {
		textKept = 0
		t.Errorf("403 Error() text drifted: got %q want %q", got, expected)
	}
	if got, expected := (&IAMClientHTTPError{
		Op: "CreateOrganization", Status: 400, Body: `{"message":"parent organization invalid"}`,
	}).Error(), `CreateOrganization returned status 400: {"message":"parent organization invalid"}`; got != expected {
		textKept = 0
		t.Errorf("non-403 Error() text drifted: got %q want %q", got, expected)
	}

	fmt.Printf("CLASSIFIER_CASES=%d\n", len(scenes))
	fmt.Printf("CLASSIFIER_MISMATCH=%d\n", mismatch)
	fmt.Printf("HTTPERR_TEXT_KEPT=%d\n", textKept)
}

// errStub2598 is a plain (untyped) transport error, mirroring what net/http returns.
type errStub2598 string

func (e errStub2598) Error() string { return string(e) }
