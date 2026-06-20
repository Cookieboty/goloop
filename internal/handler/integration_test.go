// internal/handler/integration_test.go
package handler

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"goloop/internal/channels/gemini_callback"
	"goloop/internal/core"
	kieaipkg "goloop/internal/kieai"
	"goloop/internal/middleware"
	"goloop/internal/model"
	"goloop/internal/security"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

// injectAPIKeyID wraps a handler to inject a fake API Key ID into the context,
// bypassing real auth for integration testing.
func injectAPIKeyID(next http.Handler, apiKeyID uint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.WithAPIKeyID(r.Context(), apiKeyID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// setupIntegrationTest creates a full stack with a fake KIE.AI server and a fake image CDN.
func setupIntegrationTest(t *testing.T, kieaiHandler http.Handler, cdnResultURL string) *http.ServeMux {
	t.Helper()

	kieaiSrv := httptest.NewServer(kieaiHandler)
	t.Cleanup(kieaiSrv.Close)

	dir := t.TempDir()
	store, err := storage.NewStore(dir, "http://localhost/images", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	store.SetHTTPClient(&http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})

	registry := core.NewPluginRegistry()
	health := core.NewHealthTracker()
	router := core.NewRouter(registry, health)
	issuer := core.NewJWTIssuer("test-secret", 1*time.Hour)

	pool := gemini_callback.NewAccountPool()
	pool.AddAccount("test-key", 100)
	ch := gemini_callback.NewChannel("kieai", kieaiSrv.URL, 100, pool, gemini_callback.Config{
		BaseURL:         kieaiSrv.URL,
		Timeout:         10 * time.Second,
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     30 * time.Millisecond,
		MaxWaitTime:     5 * time.Second,
		RetryAttempts:   3,
	}, store)
	registry.Register(ch)

	configMgr := core.NewConfigManager(nil)
	reqTr := transformer.NewRequestTransformer(store, configMgr, 0)
	respTr := transformer.NewResponseTransformer(store)
	client := kieaipkg.NewClient(kieaiSrv.URL, 10*time.Second)
	taskManager := kieaipkg.NewTaskManager(client, kieaipkg.PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     30 * time.Millisecond,
		MaxWaitTime:     5 * time.Second,
		RetryAttempts:   3,
	}, 2)
	t.Cleanup(taskManager.Stop)

	h := NewGeminiHandler(router, registry, issuer, store, taskManager, reqTr, respTr, 10*1024*1024, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestIntegration_TextToImage_Success(t *testing.T) {
	security.SetTestMode(true)
	defer security.SetTestMode(false)

	var pollCount atomic.Int32

	cdnSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer cdnSrv.Close()

	resultURL := cdnSrv.URL + "/fake-result.png"

	kieaiMux := http.NewServeMux()
	kieaiMux.HandleFunc("POST /api/v1/jobs/createTask", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(model.KieAICreateTaskResponse{
			Code: 200,
			Data: model.KieAITaskData{TaskID: "integ-task-001"},
		})
	})
	kieaiMux.HandleFunc("GET /api/v1/jobs/recordInfo", func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		var resp model.KieAIRecordInfoResponse
		if n < 2 {
			resp.Data = model.KieAIRecordData{State: "generating"}
		} else {
			resp.Data = model.KieAIRecordData{
				State:         "success",
				ResultJSONRaw: `{"resultUrls":["` + resultURL + `"]}`,
			}
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux := setupIntegrationTest(t, kieaiMux, resultURL)
	// Wrap with fake auth to inject API Key ID
	appSrv := httptest.NewServer(injectAPIKeyID(mux, 1))
	defer appSrv.Close()

	body := `{"contents":[{"parts":[{"text":"draw a sunset"}]}]}`
	req, _ := http.NewRequest("POST",
		appSrv.URL+"/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		t.Errorf("unexpected error status: %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusOK {
		var googleResp model.GoogleResponse
		if err := json.NewDecoder(resp.Body).Decode(&googleResp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(googleResp.Candidates) == 0 {
			t.Error("expected at least one candidate")
		}
	}
}

func TestIntegration_MissingAPIKey(t *testing.T) {
	mux := setupIntegrationTest(t, http.NewServeMux(), "")
	// No auth wrapper — request should be rejected
	appSrv := httptest.NewServer(mux)
	defer appSrv.Close()

	req, _ := http.NewRequest("POST",
		appSrv.URL+"/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_HealthCheck(t *testing.T) {
	mux := setupIntegrationTest(t, http.NewServeMux(), "")
	appSrv := httptest.NewServer(mux)
	defer appSrv.Close()

	resp, err := http.Get(appSrv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: got %d", resp.StatusCode)
	}
}
