package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/atbeta/picfast/internal/domain"
)

func TestAdminTOSStrategy(t *testing.T) {
	env, _, _, adminID := setupAdminEnv(t)
	configs := map[string]any{
		"endpoint":   "https://tos-s3-cn-beijing.volces.com",
		"region":     "cn-beijing",
		"bucket":     "picfast-test",
		"access_key": "test-ak",
		"secret_key": "test-sk",
		"url":        "",
		"link_mode":  "proxy",
	}
	body := map[string]any{
		"name":          "TOS",
		"strategy_type": "tos",
		"configs":       configs,
	}
	rec := doReq(env.Router, env.authReq(t, http.MethodPost, "/api/v1/admin/strategies", body, adminID, domain.RoleAdmin, 0))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", rec.Code, rec.Body.String())
	}
	id := int64(respDataMap(t, parseResp(t, rec))["id"].(float64))
	path := fmt.Sprintf("/api/v1/admin/strategies/%d", id)

	// Read the persisted strategy through the same API used by the edit dialog.
	rec = doReq(env.Router, env.authReq(t, http.MethodGet, path, nil, adminID, domain.RoleAdmin, 0))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d; body: %s", rec.Code, rec.Body.String())
	}
	data := respDataMap(t, parseResp(t, rec))
	if data["strategy_type"] != "tos" || nestedMap(t, data["configs"])["link_mode"] != "proxy" {
		t.Fatalf("stored strategy = %+v", data)
	}

	configs["url"] = "https://images.example.com"
	rec = doReq(env.Router, env.authReq(t, http.MethodPut, path, body, adminID, domain.RoleAdmin, 0))
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", rec.Code, rec.Body.String())
	}
	updated, err := env.DB.GetStrategyByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(updated.Configs, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["url"] != configs["url"] || saved["link_mode"] != "proxy" {
		t.Fatalf("updated config = %+v", saved)
	}

	delete(configs, "region")
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		endpoint := path
		if method == http.MethodPost {
			endpoint = "/api/v1/admin/strategies"
		}
		rec = doReq(env.Router, env.authReq(t, method, endpoint, body, adminID, domain.RoleAdmin, 0))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s without region: status = %d, want 400", method, rec.Code)
		}
	}
}
