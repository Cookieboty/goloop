// e2e/e2e_test.go
//
// 端到端测试 —— 真实调用 KIE.AI API
//
// 运行前准备：
//
//	cp .env.test.example .env.test
//	# 编辑 .env.test，填入真实的 KIEAI_API_KEY
//
// 运行命令：
//
//	go test ./e2e/... -v -timeout 180s -tags e2e
//
// 不想每次都跑，加 -run 过滤：
//
//	go test ./e2e/... -v -timeout 180s -tags e2e -run TestE2E_TextToImage
package e2e

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// loadEnv 从 .env.test 文件读取环境变量（不覆盖已有的系统环境变量）
func loadEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在时静默跳过，依赖系统环境变量
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// 不覆盖已有的环境变量
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

// requireAPIKey 获取 API Key，没有则跳过测试
func requireAPIKey(t *testing.T) string {
	t.Helper()
	if err := loadEnv("../.env.test"); err != nil {
		t.Fatalf("load .env.test: %v", err)
	}
	key := os.Getenv("KIEAI_API_KEY")
	if key == "" {
		t.Skip("KIEAI_API_KEY not set — create .env.test from .env.test.example")
	}
	return key
}

func baseURL() string {
	if u := os.Getenv("KIEAI_BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

// googleRequest 发送请求到本地 goloop 服务
func googleRequest(t *testing.T, apiKey, model string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL(), model)
	req, err := http.NewRequest("POST", url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 150 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// decodeGoogleResponse 解析响应并打印结果摘要
func decodeGoogleResponse(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	t.Logf("HTTP %d", resp.StatusCode)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, data)
	}
	return out
}

// ────────────────────────────────────────────────────────────
// 测试用例
// ────────────────────────────────────────────────────────────

// TestE2E_TextToImage 文生图：最基础的流程
func TestE2E_TextToImage(t *testing.T) {
	apiKey := requireAPIKey(t)

	resp := googleRequest(t, apiKey, "gemini-3.1-flash-image-preview", map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": "a red apple on a white table, photorealistic"},
				},
			},
		},
	})
	result := decodeGoogleResponse(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %v", resp.StatusCode, result)
	}

	candidates, _ := result["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatal("no candidates in response")
	}

	content := candidates[0].(map[string]any)["content"].(map[string]any)
	parts := content["parts"].([]any)

	imageCount := 0
	for _, p := range parts {
		part := p.(map[string]any)
		if _, ok := part["inlineData"]; ok {
			imageCount++
		}
	}

	t.Logf("parts total=%d, images=%d", len(parts), imageCount)
	if imageCount == 0 {
		t.Error("expected at least 1 inlineData image part")
	}
}

// TestE2E_TextToImage_ProModel 使用 Pro 模型
func TestE2E_TextToImage_ProModel(t *testing.T) {
	apiKey := requireAPIKey(t)

	resp := googleRequest(t, apiKey, "gemini-3-pro-image-preview", map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": "a serene mountain lake at sunrise"},
				},
			},
		},
	})
	result := decodeGoogleResponse(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %v", resp.StatusCode, result)
	}

	candidates, _ := result["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatal("no candidates in response")
	}
	t.Logf("pro model response: finishReason=%v", candidates[0].(map[string]any)["finishReason"])
}

// TestE2E_CustomAspectRatio 自定义宽高比和分辨率
func TestE2E_CustomAspectRatio(t *testing.T) {
	apiKey := requireAPIKey(t)

	resp := googleRequest(t, apiKey, "gemini-3.1-flash-image-preview", map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": "a wide panoramic forest landscape"},
				},
			},
		},
		"generationConfig": map[string]any{
			"imageConfig": map[string]any{
				"aspectRatio":  "16:9",
				"imageSize":    "1K",
				"outputFormat": "png",
			},
		},
	})
	result := decodeGoogleResponse(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %v", resp.StatusCode, result)
	}

	candidates, _ := result["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatal("no candidates")
	}
}

// TestE2E_ImageInBase64Output 验证返回的 inlineData 是合法 base64
func TestE2E_ImageInBase64Output(t *testing.T) {
	apiKey := requireAPIKey(t)

	resp := googleRequest(t, apiKey, "gemini-3.1-flash-image-preview", map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": "a simple blue circle on white background"},
				},
			},
		},
	})

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	defer resp.Body.Close()
	var googleResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&googleResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for i, candidate := range googleResp.Candidates {
		for j, part := range candidate.Content.Parts {
			if part.InlineData == nil {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				t.Errorf("candidate[%d] part[%d]: invalid base64: %v", i, j, err)
				continue
			}
			t.Logf("candidate[%d] part[%d]: mimeType=%s, size=%d bytes", i, j, part.InlineData.MimeType, len(raw))
			if len(raw) == 0 {
				t.Errorf("candidate[%d] part[%d]: empty image data", i, j)
			}
		}
	}
}

// TestE2E_InvalidAPIKey 无效 Key 应返回 401
func TestE2E_InvalidAPIKey(t *testing.T) {
	if err := loadEnv("../.env.test"); err != nil {
		t.Fatalf("load .env.test: %v", err)
	}

	resp := googleRequest(t, "invalid-key-that-does-not-exist", "gemini-3.1-flash-image-preview", map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": "test"}}},
		},
	})
	defer resp.Body.Close()

	// KIE.AI 返回 401，中间件透传为 401 或 500（取决于 KIE.AI 的响应格式）
	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for invalid API key")
	}
	t.Logf("invalid key got HTTP %d (expected 401 or 500)", resp.StatusCode)
}

// TestE2E_UnknownModel 不支持的模型应返回 400
func TestE2E_UnknownModel(t *testing.T) {
	apiKey := requireAPIKey(t)

	resp := googleRequest(t, apiKey, "gemini-unknown-model-xyz", map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": "test"}}},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown model, got %d", resp.StatusCode)
	}
}

// TestE2E_MissingAPIKey 不传 Key 应返回 401
func TestE2E_MissingAPIKey(t *testing.T) {
	body := map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": "test"}}},
		},
	}
	raw, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/v1beta/models/gemini-3.1-flash-image-preview:generateContent", baseURL())
	req, _ := http.NewRequest("POST", url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	// 不设置任何认证头

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// TestE2E_HealthCheck 健康检查端点
func TestE2E_HealthCheck(t *testing.T) {
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("health check request failed: %v — is the server running?", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	t.Logf("health response: %v", body)
}
