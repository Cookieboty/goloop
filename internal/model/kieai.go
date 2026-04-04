// internal/model/kieai.go
package model

import "encoding/json"

// --- KIE.AI Create Task ---

type KieAICreateTaskRequest struct {
	Model string     `json:"model"`
	Input KieAIInput `json:"input"`
}

type KieAIInput struct {
	Prompt string `json:"prompt"`
	// Text-to-image models (nano-banana-2, nano-banana-pro, google/nano-banana)
	ImageInput  []string `json:"image_input,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	// Edit model (google/nano-banana-edit)
	ImageURLs []string `json:"image_urls,omitempty"`
	ImageSize string   `json:"image_size,omitempty"`
	// Common
	OutputFormat string `json:"output_format,omitempty"`
}

type KieAICreateTaskResponse struct {
	Code int           `json:"code"`
	Msg  string        `json:"msg"`
	Data KieAITaskData `json:"data"`
}

type KieAITaskData struct {
	TaskID string `json:"taskId"`
}

// --- KIE.AI Poll Task ---

type KieAIRecordInfoResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data KieAIRecordData `json:"data"`
}

type KieAIRecordData struct {
	TaskID        string `json:"taskId"`
	State         string `json:"state"` // waiting/queuing/generating/success/fail
	ResultJSONRaw string `json:"resultJson,omitempty"`
	FailReason    string `json:"failReason,omitempty"`
	parsedResult  *KieAIResult
	parseErr      error
}

// ResultJSON 返回解析后的结果，如果 resultJson 为空或解析失败返回 nil
func (r *KieAIRecordData) ResultJSON() *KieAIResult {
	if r.parsedResult != nil || r.parseErr != nil {
		return r.parsedResult
	}
	if r.ResultJSONRaw == "" {
		return nil
	}
	var result KieAIResult
	if err := json.Unmarshal([]byte(r.ResultJSONRaw), &result); err != nil {
		r.parseErr = err
		return nil
	}
	r.parsedResult = &result
	return r.parsedResult
}

type KieAIResult struct {
	ResultURLs []string `json:"resultUrls"`
}
