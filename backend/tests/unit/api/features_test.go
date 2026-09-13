package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/config"
	"urara-vision/backend/internal/store/postgres"
)

func newFeaturesServer(t *testing.T, meta *fakeMeta, chatEnabled bool, token string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		CORSOrigins: []string{"http://localhost:5173"},
		ChatEnabled: chatEnabled,
		APIToken:    token,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(cfg, meta, &fakeGraphs{}, log).Routes()
}

type featuresBody struct {
	Chat struct {
		Available bool `json:"available"`
		Enabled   bool `json:"enabled"`
	} `json:"chat"`
}

func decodeFeatures(t *testing.T, body io.Reader) featuresBody {
	t.Helper()
	var got featuresBody
	if err := json.NewDecoder(body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func patchSettings(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, http.MethodPatch, "/api/v1/settings", strings.NewReader(body), "application/json")
}

func TestFeaturesTable(t *testing.T) {
	cases := []struct {
		name          string
		deployed      bool
		row           map[string]bool
		wantAvailable bool
		wantEnabled   bool
	}{
		{"not deployed", false, map[string]bool{postgres.SettingChatEnabled: true}, false, false},
		{"deployed, row missing", true, nil, true, true},
		{"deployed, row false", true, map[string]bool{postgres.SettingChatEnabled: false}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFeaturesServer(t, &fakeMeta{settings: tc.row}, tc.deployed, "")
			rec := do(t, h, http.MethodGet, "/api/v1/features", nil, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			got := decodeFeatures(t, rec.Body)
			if got.Chat.Available != tc.wantAvailable || got.Chat.Enabled != tc.wantEnabled {
				t.Errorf("chat = %+v, want available=%v enabled=%v", got.Chat, tc.wantAvailable, tc.wantEnabled)
			}
		})
	}
}

func TestFeaturesNotDeployedSkipsStore(t *testing.T) {
	meta := &fakeMeta{}
	h := newFeaturesServer(t, meta, false, "")
	do(t, h, http.MethodGet, "/api/v1/features", nil, "")
	if meta.settingReads != 0 {
		t.Errorf("settingReads = %d, want 0", meta.settingReads)
	}
}

func TestPatchSettingsRejectsBadBodies(t *testing.T) {
	cases := []struct {
		name     string
		deployed bool
		body     string
		want     int
	}{
		{"empty object", true, `{}`, http.StatusBadRequest},
		{"unknown field", true, `{"other":1}`, http.StatusBadRequest},
		{"enable while not deployed", false, `{"chatEnabled":true}`, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &fakeMeta{}
			h := newFeaturesServer(t, meta, tc.deployed, "")
			rec := patchSettings(t, h, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.want, rec.Body)
			}
			if len(meta.settingWrites) != 0 {
				t.Errorf("settingWrites = %v, want none", meta.settingWrites)
			}
		})
	}
}

func TestPatchSettingsConflictMessage(t *testing.T) {
	h := newFeaturesServer(t, &fakeMeta{}, false, "")
	rec := patchSettings(t, h, `{"chatEnabled":true}`)
	if !strings.Contains(rec.Body.String(), "chat is not deployed") {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestPatchSettingsDisables(t *testing.T) {
	meta := &fakeMeta{}
	h := newFeaturesServer(t, meta, true, "")
	rec := patchSettings(t, h, `{"chatEnabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	got := decodeFeatures(t, rec.Body)
	if !got.Chat.Available || got.Chat.Enabled {
		t.Errorf("chat = %+v, want available and not enabled", got.Chat)
	}
	if v, ok := meta.settingWrites[postgres.SettingChatEnabled]; !ok || v {
		t.Errorf("settingWrites = %v, want %s=false", meta.settingWrites, postgres.SettingChatEnabled)
	}
}

func TestFeaturesStoreErrorIs500(t *testing.T) {
	for _, tc := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/v1/features", ""},
		{http.MethodPatch, "/api/v1/settings", `{"chatEnabled":false}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			h := newFeaturesServer(t, &fakeMeta{errSetting: errBoom}, true, "")
			rec := do(t, h, tc.method, tc.target, strings.NewReader(tc.body), "application/json")
			if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "internal error") {
				t.Errorf("status = %d, body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestFeaturesRequireToken(t *testing.T) {
	h := newFeaturesServer(t, &fakeMeta{}, true, testToken)
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/features"},
		{http.MethodPatch, "/api/v1/settings"},
	} {
		if rec := req(t, h, tc.method, tc.target, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.target, rec.Code)
		}
	}
}
