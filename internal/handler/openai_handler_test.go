package handler

import (
	"net/http"
	"testing"
)

func TestShouldFallbackOnStatus(t *testing.T) {
	retryCodes := map[int]struct{}{403: {}, 502: {}, 524: {}}
	fallbackCodes := map[int]struct{}{400: {}}

	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusOK, false},
		{http.StatusNotFound, false},
		{http.StatusBadRequest, true},   // 400 via fallbackCodes
		{http.StatusUnauthorized, true}, // 401 built-in
		{http.StatusRequestTimeout, true},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{403, true}, // in retryCodes
		{524, true}, // in retryCodes
	}
	for _, tt := range tests {
		if got := shouldFallbackOnStatus(tt.status, retryCodes, fallbackCodes); got != tt.want {
			t.Errorf("shouldFallbackOnStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestShouldFallbackOnStatus_EmptyFallbackCodes(t *testing.T) {
	retryCodes := map[int]struct{}{403: {}}
	fallbackCodes := map[int]struct{}{} // 400 not configured -> no fallback

	if shouldFallbackOnStatus(http.StatusBadRequest, retryCodes, fallbackCodes) {
		t.Error("shouldFallbackOnStatus(400) = true with empty fallbackCodes, want false")
	}
}

func TestShouldRecordChannelFailure(t *testing.T) {
	retryCodes := map[int]struct{}{403: {}, 502: {}, 524: {}}
	fallbackCodes := map[int]struct{}{400: {}}

	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusBadRequest, false}, // fallback-only, must not hurt channel health
		{403, false},                   // retryCoded — already retried in-channel
		{502, false},                   // retryCoded
		{http.StatusInternalServerError, true},
		{http.StatusUnauthorized, true},
		{http.StatusTooManyRequests, true},
		{http.StatusRequestTimeout, true},
	}
	for _, tt := range tests {
		if got := shouldRecordChannelFailure(tt.status, retryCodes, fallbackCodes); got != tt.want {
			t.Errorf("shouldRecordChannelFailure(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
