package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type dashboardLayoutItemResponse struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

type dashboardLayoutResponse struct {
	Version int                           `json:"version"`
	Modules []dashboardLayoutItemResponse `json:"modules"`
}

func requestDashboardLayout(t *testing.T, method, path string, body interface{}, master bool) (*httptest.ResponseRecorder, dashboardLayoutResponse) {
	t.Helper()
	h, sessionsSvc, _ := newAdminOnboardingTestHandler(t, nil)

	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal dashboard layout request: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	req := newAdminRequestWithSession(t, sessionsSvc, method, path, master)
	req.Body = http.NoBody
	if body != nil {
		req = httptest.NewRequest(method, path, requestBody)
		sessionReq := newAdminRequestWithSession(t, sessionsSvc, method, path, master)
		for _, cookie := range sessionReq.Cookies() {
			req.AddCookie(cookie)
		}
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	switch method {
	case http.MethodGet:
		h.RequireAuth(h.GetDashboardLayout).ServeHTTP(rr, req)
	case http.MethodPut:
		h.RequireMasterAuth(h.SaveDashboardLayout).ServeHTTP(rr, req)
	case http.MethodDelete:
		h.RequireMasterAuth(h.ResetDashboardLayout).ServeHTTP(rr, req)
	default:
		t.Fatalf("unsupported method %s", method)
	}

	var response dashboardLayoutResponse
	if rr.Code >= 200 && rr.Code < 300 && rr.Body.Len() > 0 {
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode dashboard layout response: %v; body=%s", err, rr.Body.String())
		}
	}
	return rr, response
}

func TestDashboardLayoutDefaultIsCompleteAndValid(t *testing.T) {
	rr, layout := requestDashboardLayout(t, http.MethodGet, "/admin/api/dashboard/layout", nil, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if layout.Version != 1 {
		t.Fatalf("version = %d, want 1", layout.Version)
	}
	if len(layout.Modules) != 17 {
		t.Fatalf("default module count = %d, want 17", len(layout.Modules))
	}

	seen := make(map[string]bool, len(layout.Modules))
	for _, module := range layout.Modules {
		if module.ID == "" || seen[module.ID] {
			t.Fatalf("default layout contains missing or duplicate module id %q", module.ID)
		}
		seen[module.ID] = true
		if module.X < 0 || module.Y < 0 || module.W < 1 || module.H < 1 || module.X+module.W > 12 {
			t.Fatalf("module %q is outside the 12-column grid: %+v", module.ID, module)
		}
	}
}

func TestDashboardLayoutSavePersistsValidArrangement(t *testing.T) {
	h, sessionsSvc, _ := newAdminOnboardingTestHandler(t, nil)
	getReq := newAdminRequestWithSession(t, sessionsSvc, http.MethodGet, "/admin/api/dashboard/layout", true)
	getRR := httptest.NewRecorder()
	h.RequireAuth(h.GetDashboardLayout).ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get default status = %d; body=%s", getRR.Code, getRR.Body.String())
	}

	var layout dashboardLayoutResponse
	if err := json.Unmarshal(getRR.Body.Bytes(), &layout); err != nil {
		t.Fatalf("decode default layout: %v", err)
	}
	if len(layout.Modules) < 2 {
		t.Fatal("default layout does not contain enough modules to rearrange")
	}
	layout.Modules[0].X, layout.Modules[1].X = layout.Modules[1].X, layout.Modules[0].X
	for i := range layout.Modules {
		layout.Modules[i].Y += 5
	}

	payload, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	putReq := newAdminRequestWithSession(t, sessionsSvc, http.MethodPut, "/admin/api/dashboard/layout", true)
	putReq.Body = io.NopCloser(bytes.NewReader(payload))
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	h.RequireMasterAuth(h.SaveDashboardLayout).ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d; body=%s", putRR.Code, http.StatusOK, putRR.Body.String())
	}

	reloadReq := newAdminRequestWithSession(t, sessionsSvc, http.MethodGet, "/admin/api/dashboard/layout", true)
	reloadRR := httptest.NewRecorder()
	h.RequireAuth(h.GetDashboardLayout).ServeHTTP(reloadRR, reloadReq)
	if reloadRR.Code != http.StatusOK {
		t.Fatalf("reload status = %d; body=%s", reloadRR.Code, reloadRR.Body.String())
	}

	var reloaded dashboardLayoutResponse
	if err := json.Unmarshal(reloadRR.Body.Bytes(), &reloaded); err != nil {
		t.Fatalf("decode reloaded layout: %v", err)
	}
	if reloaded.Modules[0].ID != layout.Modules[0].ID || reloaded.Modules[0].X != layout.Modules[0].X {
		t.Fatalf("saved layout was not persisted: got %+v want %+v", reloaded.Modules[0], layout.Modules[0])
	}
	if reloaded.Modules[0].Y != 0 {
		t.Fatalf("saved layout was not auto-packed upward: first module y = %d, want 0", reloaded.Modules[0].Y)
	}
}

func TestDashboardLayoutRejectsUnsafeAndUnknownModules(t *testing.T) {
	h, sessionsSvc, _ := newAdminOnboardingTestHandler(t, nil)
	getReq := newAdminRequestWithSession(t, sessionsSvc, http.MethodGet, "/admin/api/dashboard/layout", true)
	getRR := httptest.NewRecorder()
	h.RequireAuth(h.GetDashboardLayout).ServeHTTP(getRR, getReq)

	var layout dashboardLayoutResponse
	if err := json.Unmarshal(getRR.Body.Bytes(), &layout); err != nil {
		t.Fatalf("decode default layout: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*dashboardLayoutResponse)
	}{
		{name: "below minimum width", mutate: func(layout *dashboardLayoutResponse) { layout.Modules[0].W = 1 }},
		{name: "outside grid", mutate: func(layout *dashboardLayoutResponse) { layout.Modules[0].X = 12 }},
		{name: "unknown module", mutate: func(layout *dashboardLayoutResponse) { layout.Modules[0].ID = "unknown-module" }},
		{name: "duplicate module", mutate: func(layout *dashboardLayoutResponse) { layout.Modules[0].ID = layout.Modules[1].ID }},
		{name: "overlapping module", mutate: func(layout *dashboardLayoutResponse) {
			layout.Modules[0].X = layout.Modules[1].X
			layout.Modules[0].Y = layout.Modules[1].Y
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := dashboardLayoutResponse{Version: layout.Version, Modules: append([]dashboardLayoutItemResponse(nil), layout.Modules...)}
			tt.mutate(&candidate)
			payload, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("marshal candidate: %v", err)
			}
			req := newAdminRequestWithSession(t, sessionsSvc, http.MethodPut, "/admin/api/dashboard/layout", true)
			req.Body = io.NopCloser(bytes.NewReader(payload))
			rr := httptest.NewRecorder()
			h.RequireMasterAuth(h.SaveDashboardLayout).ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
		})
	}
}

func TestDashboardLayoutMutationRequiresMaster(t *testing.T) {
	rr, _ := requestDashboardLayout(t, http.MethodPut, "/admin/api/dashboard/layout", dashboardLayoutResponse{Version: 1}, false)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestDashboardLayoutResetRestoresDefault(t *testing.T) {
	rr, layout := requestDashboardLayout(t, http.MethodDelete, "/admin/api/dashboard/layout", nil, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if layout.Version != 1 || len(layout.Modules) != 17 {
		t.Fatalf("reset response is not the complete default layout: %+v", layout)
	}
}
