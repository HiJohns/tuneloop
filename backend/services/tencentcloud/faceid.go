package tencentcloud

import "errors"

// ErrNotConfigured is returned when Tencent Cloud credentials are missing.
var ErrNotConfigured = errors.New("face verify not configured: TENCENTCLOUD_SECRET_ID/KEY not set")

// FaceVerifyProvider abstracts face verification operations so the handler
// can be tested with a fake implementation without calling the real Tencent API.
type FaceVerifyProvider interface {
	// GetToken requests a face-verification session token from Tencent Cloud.
	// name: user's real name; idCard: 18-digit ID card number.
	// Returns a BizToken to be passed to the frontend plugin, or an error.
	GetToken(name, idCard string) (bizToken string, err error)

	// GetResult queries the result of a face-verification session.
	// Returns passed (whether verification succeeded), similarity score, or error.
	GetResult(bizToken string) (passed bool, similarity float64, err error)
}

// NullFaceProvider is returned when Tencent Cloud credentials are not configured.
// All methods return a clear "not configured" error.
type NullFaceProvider struct{}

func (NullFaceProvider) GetToken(_, _ string) (string, error) {
	return "", ErrNotConfigured
}

func (NullFaceProvider) GetResult(_ string) (bool, float64, error) {
	return false, 0, ErrNotConfigured
}
