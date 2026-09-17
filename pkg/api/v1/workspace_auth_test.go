package apiv1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beam-cloud/beta9/pkg/auth"
	"github.com/beam-cloud/beta9/pkg/types"
	"github.com/labstack/echo/v4"
)

// FlowState fork: the create-workspace route used to be registered without an
// auth wrapper, so a request carrying no token reached CreateWorkspace as a
// plain echo.Context and panicked on cc.AuthInfo. No Recover middleware here,
// so a regression fails this test with that panic.
func TestCreateWorkspaceWithoutTokenIsUnauthorized(t *testing.T) {
	e := echo.New()
	NewWorkspaceGroup(e.Group("/api/v1/workspace"), nil, nil, nil, types.AppConfig{})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/workspace", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateWorkspaceWithWorkspaceTokenIsUnauthorized(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(&auth.HttpAuthContext{Context: c, AuthInfo: &auth.AuthInfo{
				Token:     &types.Token{TokenType: types.TokenTypeWorkspacePrimary},
				Workspace: &types.Workspace{},
			}})
		}
	})
	NewWorkspaceGroup(e.Group("/api/v1/workspace"), nil, nil, nil, types.AppConfig{})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/workspace", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
