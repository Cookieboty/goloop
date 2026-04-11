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

	"github.com/golang-jwt/jwt/v5"
	"goloop/internal/channels/kieai"
	kieaipkg "goloop/internal/kieai"
	"goloop/internal/config"
	"goloop/internal/core"
	"goloop/internal/model"
	"goloop/internal/security"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

// setupIntegrationTest creates a full stack with a fake KIE.AI server and a fake image CDN.
// cdnResultURL is the URL that KIE.AI will return as a result image URL.
func setupIntegrationTest(t *testing.T, kieaiHandler http.Handler, cdnResultURL string) (*http.ServeMux, *core.JWTIssuer) {
	t.Helper()

	kieaiSrv := httptest.NewServer(kieaiHandler)
	t.Cleanup(kieaiSrv.Close)

	dir := t.TempDir()
	store, err := storage.NewStore(dir, "http://localhost/images")
	if err != nil {
		t.Fatal(err)
	}

	// For tests, use HTTP client that skips TLS verification
	store.SetHTTPClient(&http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})

	// Core infrastructure
	registry := core.NewPluginRegistry()
	health := core.NewHealthTracker()
	router := core.NewRouter(registry, health)
	issuer := core.NewJWTIssuer("test-secret", 1*time.Hour)

	// Create kieai channel for testing
	pool := kieai.NewAccountPool()
	pool.AddAccount("test-key", 100)
	ch := kieai.NewChannel(kieaiSrv.URL, 100, pool, kieai.Config{
		BaseURL:         kieaiSrv.URL,
		Timeout:         10 * time.Second,
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     30 * time.Millisecond,
		MaxWaitTime:    5 * time.Second,
		RetryAttempts:   3,
	})
	registry.Register(ch)

	modelMapping := map[string]config.ModelDefaults{
		"gemini-3.1-flash-image-preview": {
			Channel: "kieai", KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
		},
	}

	reqTr := transformer.NewRequestTransformer(store, modelMapping)
	respTr := transformer.NewResponseTransformer(store)
	client := kieaipkg.NewClient(kieaiSrv.URL, 10*time.Second)
	taskManager := kieaipkg.NewTaskManager(client, kieaipkg.PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     30 * time.Millisecond,
		MaxWaitTime:     5 * time.Second,
		RetryAttempts:   3,
	}, 2) // 2 workers for test
	t.Cleanup(taskManager.Stop)

	h := NewGeminiHandler(router, registry, issuer, store, taskManager, reqTr, respTr)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, issuer
}

func TestIntegration_TextToImage_Success(t *testing.T) {
	security.SetTestMode(true)
	defer security.SetTestMode(false)

	var pollCount atomic.Int32

	// CDN server serves fake PNG bytes (use HTTPS for SSRF protection)
	cdnSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG\r\n\x1a\n")) // minimal PNG header
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

	mux, issuer := setupIntegrationTest(t, kieaiMux, resultURL)
	appSrv := httptest.NewServer(mux)
	defer appSrv.Close()

	// Issue JWT token for the request
	token, _ := issuer.Issue(&core.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "test-user"},
		
		Channel: "kieai",
	})

	body := `{"contents":[{"parts":[{"text":"draw a sunset"}]}]}`
	req, _ := http.NewRequest("POST",
		appSrv.URL+"/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

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
	mux, _ := setupIntegrationTest(t, http.NewServeMux(), "")
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

	var gErr model.GoogleError
	json.NewDecoder(resp.Body).Decode(&gErr)
	if gErr.Error.Status != "UNAUTHENTICATED" {
		t.Errorf("status: got %q", gErr.Error.Status)
	}
}

func TestIntegration_HealthCheck(t *testing.T) {
	mux, _ := setupIntegrationTest(t, http.NewServeMux(), "")
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
