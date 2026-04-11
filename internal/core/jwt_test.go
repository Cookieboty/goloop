package core

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

func TestJWTLifecycle(t *testing.T) {
    issuer := NewJWTIssuer("test-secret", 1*time.Hour)

    claims := &JWTClaims{
        RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123"},
        Channel:           "kieai",
    }
    token, err := issuer.Issue(claims)
    if err != nil { t.Fatalf("Issue error: %v", err) }
    if token == "" { t.Fatal("token should not be empty") }

    parsed, err := issuer.Validate(token)
    if err != nil { t.Fatalf("Validate error: %v", err) }
    if parsed.Subject != "user-123" { t.Errorf("subject mismatch") }
    if parsed.Channel != "kieai" { t.Errorf("channel mismatch") }

    // Invalid token
    _, err = issuer.Validate("invalid")
    if err == nil { t.Errorf("expected error for invalid token") }
}

func TestJWTMiddleware_MissingToken(t *testing.T) {
    issuer := NewJWTIssuer("secret", 1*time.Hour)
    var nextCalled bool

    mw := NewJWTMiddleware(issuer, func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request) {
        nextCalled = true
    })

    req := httptest.NewRequest("POST", "/", nil)
    rr := httptest.NewRecorder()
    mw.ServeHTTP(rr, req)

    if rr.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rr.Code)
    }
    if nextCalled { t.Errorf("next should not be called without token") }
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
    issuer := NewJWTIssuer("secret", 1*time.Hour)
    token, _ := issuer.Issue(&JWTClaims{
        RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"},
    })

    var captured *JWTClaims
    var nextCalled bool
    mw := NewJWTMiddleware(issuer, func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request) {
        captured = claims
        nextCalled = true
    })

    req := httptest.NewRequest("POST", "/", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rr := httptest.NewRecorder()
    mw.ServeHTTP(rr, req)

    if !nextCalled || captured == nil || captured.Subject != "u1" {
        t.Errorf("next not called correctly: nextCalled=%v captured=%v", nextCalled, captured)
    }
}