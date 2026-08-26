package pds

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alibabacloud-go/tea/dara"
)

func TestIsCursorRejectedCode(t *testing.T) {
	rejected := []string{
		"InvalidCursor",
		"CursorExpired",
		"CursorNotFound",
		"NotFound.Cursor", // production OpenAPI 2022-03-01 code (HTTP 404)
		"notfound.cursor",
		"InvalidParameter.Cursor",
	}
	for _, code := range rejected {
		if !isCursorRejectedCode(code) {
			t.Errorf("isCursorRejectedCode(%q) = false, want true", code)
		}
	}
	accepted := []string{
		"",
		"NotFound",
		"InvalidParameter.DriveId",
		"InvalidAccessToken",
		"cursor", // mentions cursor but carries no rejection marker
	}
	for _, code := range accepted {
		if isCursorRejectedCode(code) {
			t.Errorf("isCursorRejectedCode(%q) = true, want false", code)
		}
	}
}

func TestIsCursorExpiredErr(t *testing.T) {
	productionSDKErr := &dara.SDKError{
		Code:       dara.String("NotFound.Cursor"),
		StatusCode: dara.Int(404),
		Message:    dara.String("The resource cursor cannot be found. cursor is not exist"),
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"api NotFound.Cursor", &pdsAPIError{Status: 404, Code: "NotFound.Cursor", Message: "cursor is not exist"}, true},
		{"api InvalidCursor", &pdsAPIError{Status: 400, Code: "InvalidCursor", Message: "cursor expired"}, true},
		{"api unrelated code", &pdsAPIError{Status: 404, Code: "NotFound", Message: "file not found"}, false},
		{"sdk NotFound.Cursor", productionSDKErr, true},
		{"wrapped sdk NotFound.Cursor", fmt.Errorf("pds list delta: %w", productionSDKErr), true},
		{"sdk unrelated code", &dara.SDKError{Code: dara.String("InvalidParameter.DriveId"), StatusCode: dara.Int(400)}, false},
		{"cursorExpiredError passthrough", &cursorExpiredError{err: errors.New("boom")}, true},
		{"message fallback", errors.New("The resource cursor cannot be found. cursor is not exist"), true},
		{"network error", errors.New("connection reset by peer"), false},
	}
	for _, tc := range cases {
		if got := isCursorExpiredErr(tc.err); got != tc.want {
			t.Errorf("%s: isCursorExpiredErr = %v, want %v", tc.name, got, tc.want)
		}
	}
}
