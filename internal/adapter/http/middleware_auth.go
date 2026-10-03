package http

import (
	"context"
	"net/http"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

type userContextKey struct{}

// WithUser associates the authenticated user with the context.
func WithUser(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// UserFromContext retrieves the authenticated user from the context.
func UserFromContext(ctx context.Context) *domain.User {
	if val := ctx.Value(userContextKey{}); val != nil {
		if u, ok := val.(*domain.User); ok {
			return u
		}
	}
	return nil
}

// Authenticator verifies user session tokens via Bearer header or cookie.
func Authenticator(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authSvc == nil {
				next.ServeHTTP(w, r)
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				writeJSONError(w, http.StatusUnauthorized, "authorization bearer token or session cookie required")
				return
			}

			user, ok := authSvc.ValidateToken(token)
			if !ok || user == nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid or expired session token")
				return
			}

			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole checks that the authenticated user possesses one of the allowed roles.
func RequireRole(allowedRoles ...domain.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			for _, role := range allowedRoles {
				if user.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeJSONError(w, http.StatusForbidden, "insufficient privileges for this operation")
		})
	}
}
