// User administration handlers.
package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
)

const goodPassword = "a-long-enough-password"

func asAdmin(t *testing.T, h http.Handler, method, target, body string) (int, string) {
	t.Helper()
	rec := asRole(t, h, auth.RoleAdmin, method, target, strings.NewReader(body))
	return rec.Code, rec.Body.String()
}

func TestUsersList(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)
	meta.passwords = map[string]string{"admin": "secret-hash"}

	code, body := asAdmin(t, h, http.MethodGet, "/api/v1/users", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, body)
	}
	var out struct {
		Users []struct{ Username string } `json:"users"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Users) != 3 || out.Users[0].Username != "admin" || out.Users[2].Username != "viewer" {
		t.Errorf("users = %+v", out.Users)
	}
	if strings.Contains(body, "secret-hash") || strings.Contains(strings.ToLower(body), "password") {
		t.Errorf("response leaks a password: %s", body)
	}
}

func TestUsersCreate(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	code, body := asAdmin(t, h, http.MethodPost, "/api/v1/users",
		`{"username":"Vera","displayName":"Vera","password":"`+goodPassword+`","role":"viewer"}`)
	if code != http.StatusCreated || !strings.Contains(body, `"username":"vera"`) {
		t.Fatalf("status = %d: %s", code, body)
	}
	if strings.Contains(body, meta.passwords["id-vera"]) {
		t.Error("response carries the hash")
	}
	if !auth.VerifyPassword(meta.passwords["id-vera"], goodPassword) {
		t.Error("stored hash does not verify")
	}
}

func TestUsersCreateRejects(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	for _, c := range []struct{ name, body, want string }{
		{"username", `{"username":"x","password":"` + goodPassword + `","role":"viewer"}`, "username must be"},
		{"role", `{"username":"vera","password":"` + goodPassword + `","role":"owner"}`, "unknown role"},
		{"password", `{"username":"vera","password":"short","role":"viewer"}`, "password must be"},
		{"displayName", `{"username":"vera","displayName":"` + strings.Repeat("é", 101) + `","password":"` + goodPassword + `","role":"viewer"}`, "displayName"},
	} {
		code, body := asAdmin(t, h, http.MethodPost, "/api/v1/users", c.body)
		if code != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Errorf("%s: status = %d, body = %s", c.name, code, body)
		}
	}
}

func TestUsersCreateDuplicateIs409(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	code, body := asAdmin(t, h, http.MethodPost, "/api/v1/users",
		`{"username":"viewer","password":"`+goodPassword+`","role":"viewer"}`)
	if code != http.StatusConflict || !strings.Contains(body, "username already exists") {
		t.Errorf("status = %d: %s", code, body)
	}
}

func TestUsersPatch(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	code, body := asAdmin(t, h, http.MethodPatch, "/api/v1/users/viewer", `{"role":"creator","displayName":"Vee"}`)
	if code != http.StatusOK || meta.users["viewer"].Role != "creator" || meta.users["viewer"].DisplayName != "Vee" {
		t.Errorf("status = %d: %s", code, body)
	}
}

func TestUsersPatchRejects(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	for _, c := range []struct {
		name, target, body string
		code               int
		want               string
	}{
		{"empty", "/api/v1/users/viewer", `{}`, http.StatusBadRequest, "nothing to change"},
		{"role", "/api/v1/users/viewer", `{"role":"owner"}`, http.StatusBadRequest, "unknown role"},
		{"username", "/api/v1/users/viewer", `{"username":"x"}`, http.StatusBadRequest, "unknown field"},
		{"unknown", "/api/v1/users/nobody", `{"role":"viewer"}`, http.StatusNotFound, "not found"},
		{"last admin", "/api/v1/users/admin", `{"role":"viewer"}`, http.StatusConflict, "at least one admin must remain"},
	} {
		code, body := asAdmin(t, h, http.MethodPatch, c.target, c.body)
		if code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("%s: status = %d, body = %s", c.name, code, body)
		}
	}
}

func TestUsersResetPasswordRevokesSessions(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	code, body := asAdmin(t, h, http.MethodPost, "/api/v1/users/viewer/password", `{"password":"`+goodPassword+`"}`)
	if code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", code, body)
	}
	if len(meta.sessionsDeletedFor) != 1 || meta.sessionsDeletedFor[0] != [2]string{"viewer", ""} {
		t.Errorf("DeleteUserSessions calls = %v", meta.sessionsDeletedFor)
	}
	if !auth.VerifyPassword(meta.passwords["viewer"], goodPassword) {
		t.Error("password not set")
	}

	if code, body := asAdmin(t, h, http.MethodPost, "/api/v1/users/viewer/password", `{"password":"short"}`); code != http.StatusBadRequest {
		t.Errorf("weak password: status = %d: %s", code, body)
	}
}

func TestUsersDelete(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	if code, body := asAdmin(t, h, http.MethodDelete, "/api/v1/users/viewer", ""); code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", code, body)
	}
	if _, ok := meta.users["viewer"]; ok {
		t.Error("user still there")
	}
	if code, _ := asAdmin(t, h, http.MethodDelete, "/api/v1/users/viewer", ""); code != http.StatusNotFound {
		t.Errorf("second delete: status = %d, want 404", code)
	}
}

func TestUsersSelfDeleteIs409BeforeTheStore(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	code, body := asAdmin(t, h, http.MethodDelete, "/api/v1/users/admin", "")
	if code != http.StatusConflict || !strings.Contains(body, "you cannot delete your own account") {
		t.Errorf("status = %d: %s", code, body)
	}
	if meta.deleteUserCalls != 0 {
		t.Error("store was called")
	}
}

func TestUsersDeleteLastAdminIs409(t *testing.T) {
	meta := &fakeMeta{}
	meta.addUser(model.User{ID: "a1", Username: "root", Role: "admin"})
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodDelete, "/api/v1/users/a1", nil, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "at least one admin must remain") {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

func TestUsersPermissionViewerRefused(t *testing.T) {
	h := roleServer(t, &fakeMeta{})
	for _, c := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodPost, "/api/v1/users", `{}`},
		{http.MethodPatch, "/api/v1/users/creator", `{"role":"admin"}`},
		{http.MethodPost, "/api/v1/users/creator/password", `{}`},
		{http.MethodDelete, "/api/v1/users/creator", ""},
	} {
		rec := asRole(t, h, auth.RoleViewer, c.method, c.target, strings.NewReader(c.body))
		wantCode(t, c.method+" "+c.target, rec, http.StatusForbidden)
	}
}
