// internal/model/kieai.go
package model

// --- KIE.AI Create Task ---

type KieAICreateTaskRequest struct {
	Model string     `json:"model"`
	Input KieAIInput `json:"input"`
}

type KieAIInput struct {
	Prompt       string   `json:"prompt"`
	ImageInput   []string `json:"image_input,omitempty"`
	AspectRatio  string   `json:"aspect_ratio,omitempty"`
	Resolution   string   `json:"resolution,omitempty"`
	OutputFormat string   `json:"output_format,omitempty"`
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
	TaskID     string       `json:"taskId"`
	Status     string       `json:"status"` // waiting/queuing/generating/success/fail
	ResultJSON *KieAIResult `json:"resultJson,omitempty"`
	FailReason string       `json:"failReason,omitempty"`
}

type KieAIResult struct {
	ResultURLs []string `json:"resultUrls"`
}
