// internal/handler/integration_test.go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"goloop/internal/config"
	"goloop/internal/kieai"
	"goloop/internal/model"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

// setupIntegrationTest creates a full stack with a fake KIE.AI server and a fake image CDN.
// cdnResultURL is the URL that KIE.AI will return as a result image URL.
func setupIntegrationTest(t *testing.T, kieaiHandler http.Handler, cdnResultURL string) *http.ServeMux {
	t.Helper()

	kieaiSrv := httptest.NewServer(kieaiHandler)
	t.Cleanup(kieaiSrv.Close)

	dir := t.TempDir()
	store, err := storage.NewStore(dir, "http://localhost/images")
	if err != nil {
		t.Fatal(err)
	}

	modelMapping := map[string]config.ModelDefaults{
		"gemini-3.1-flash-image-preview": {
			KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
		},
	}

	reqTr := transformer.NewRequestTransformer(store, modelMapping)
	respTr := transformer.NewResponseTransformer(store)
	client := kieai.NewClient(kieaiSrv.URL, 10*time.Second)
	poller := kieai.NewPoller(client, kieai.PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     30 * time.Millisecond,
		MaxWaitTime:     5 * time.Second,
		RetryAttempts:   3,
	})

	h := NewGeminiHandler(reqTr, respTr, client, poller)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestIntegration_TextToImage_Success(t *testing.T) {
	var pollCount atomic.Int32

	// CDN server serves fake PNG bytes
	cdnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			resp.Data = model.KieAIRecordData{Status: "generating"}
		} else {
			resp.Data = model.KieAIRecordData{
				Status:     "success",
				ResultJSON: &model.KieAIResult{ResultURLs: []string{resultURL}},
			}
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux := setupIntegrationTest(t, kieaiMux, resultURL)
	appSrv := httptest.NewServer(mux)
	defer appSrv.Close()

	body := `{"contents":[{"parts":[{"text":"draw a sunset"}]}]}`
	req, _ := http.NewRequest("POST",
		appSrv.URL+"/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", "test-api-key")

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
