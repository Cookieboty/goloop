package core

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "net/http"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type jwtContextKey string

const contextKeyClaims jwtContextKey = "jwt_claims"

// JWTClaims represents the claims embedded in the JWT.
type JWTClaims struct {
    jwt.RegisteredClaims
    Channel string `json:"channel,omitempty"`
    Quota   int64  `json:"quota,omitempty"`
}

// JWTIssuer creates and validates JWTs.
type JWTIssuer struct {
    secret []byte
    expiry time.Duration
}

func NewJWTIssuer(secret string, expiry time.Duration) *JWTIssuer {
    return &JWTIssuer{secret: []byte(secret), expiry: expiry}
}

func (j *JWTIssuer) Issue(claims *JWTClaims) (string, error) {
    if claims.Subject == "" {
        return "", errors.New("jwt: subject is required")
    }
    // Only set default expiry if caller did not explicitly set ExpiresAt.
    // Pass a zero-value *jwt.NumericDate (not nil) to opt out of expiry.
    if claims.ExpiresAt == nil && j.expiry > 0 {
        claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(j.expiry))
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(j.secret)
}

func (j *JWTIssuer) Validate(tokenString string) (*JWTClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return j.secret, nil
    })
    if err != nil {
        return nil, fmt.Errorf("jwt: validate failed: %w", err)
    }
    claims, ok := token.Claims.(*JWTClaims)
    if !ok || !token.Valid {
        return nil, errors.New("jwt: invalid token")
    }
    return claims, nil
}

// JWTMiddleware validates incoming JWTs and injects claims into context.
type JWTMiddleware struct {
    issuer *JWTIssuer
    next   func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)
}

func NewJWTMiddleware(issuer *JWTIssuer, next func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)) *JWTMiddleware {
    return &JWTMiddleware{issuer: issuer, next: next}
}

func (m *JWTMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		slog.Debug("jwt middleware: no bearer token")
		mask := func(v string) string {
			if len(v) > 16 {
				return v[:8] + "..." + v[len(v)-4:]
			}
			return "***"
		}
		slog.Warn("jwt missing", "path", r.URL.Path, "auth", mask(r.Header.Get("Authorization")))
		writeError(w, http.StatusUnauthorized, "JWT token required", "UNAUTHENTICATED")
		return
	}
	claims, err := m.issuer.Validate(token)
	if err != nil {
		mask := func(v string) string {
			if len(v) > 16 {
				return v[:8] + "..." + v[len(v)-4:]
			}
			return "***"
		}
		slog.Warn("jwt invalid", "path", r.URL.Path, "auth", mask(r.Header.Get("Authorization")), "err", err)
		writeError(w, http.StatusUnauthorized, "invalid or expired token", "UNAUTHENTICATED")
		return
	}
    ctx := context.WithValue(r.Context(), contextKeyClaims, claims)
    m.next(ctx, claims, w, r.WithContext(ctx))
}

func extractBearerToken(r *http.Request) string {
    if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}

func GetClaims(ctx context.Context) *JWTClaims {
    if claims, ok := ctx.Value(contextKeyClaims).(*JWTClaims); ok {
        return claims
    }
    return nil
}

func writeError(w http.ResponseWriter, code int, message, status string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":%q}}`, code, message, status)
}