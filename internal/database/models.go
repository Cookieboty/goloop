package database

import (
	"time"
)

// Channel 渠道配置表
type Channel struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	Name                  string    `gorm:"uniqueIndex;not null" json:"name"`
	Type                  string    `gorm:"not null" json:"type"` // gemini_callback, gemini_openai, gemini_original, openai_original, openai_callback
	BaseURL               string    `gorm:"not null" json:"base_url"`
	Weight                int       `gorm:"default:100" json:"weight"`
	TimeoutSeconds        int       `gorm:"default:60" json:"timeout_seconds"`
	InitialIntervalSeconds *int     `json:"initial_interval_seconds,omitempty"` // 仅 callback 类型使用
	MaxIntervalSeconds    *int     `json:"max_interval_seconds,omitempty"` // 仅 callback 类型使用
	MaxWaitTimeSeconds    *int     `json:"max_wait_time_seconds,omitempty"` // 仅 callback 类型使用
	RetryAttempts         *int     `json:"retry_attempts,omitempty"` // 仅 callback 类型使用
	ProbeModel            *string  `json:"probe_model,omitempty"` // 仅 openai 类型使用
	Enabled               bool     `gorm:"default:true" json:"enabled"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	
	Accounts      []Account      `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"accounts,omitempty"`
	ModelMappings []ModelMapping `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"model_mappings,omitempty"`
}

// Account 账号池表
type Account struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ChannelID uint      `gorm:"not null;index" json:"channel_id"`
	APIKey    string    `gorm:"not null" json:"api_key"`
	Weight    int       `gorm:"default:100" json:"weight"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	Channel Channel `gorm:"foreignKey:ChannelID" json:"-"`
}

// ModelMapping 模型转换映射表
type ModelMapping struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ChannelID   uint      `gorm:"not null;uniqueIndex:idx_channel_source" json:"channel_id"`
	SourceModel string    `gorm:"not null;uniqueIndex:idx_channel_source" json:"source_model"`
	TargetModel string    `gorm:"not null" json:"target_model"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	
	Channel Channel `gorm:"foreignKey:ChannelID" json:"-"`
}

// APIKey 客户端 API Key 表
type APIKey struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	Key                string     `gorm:"uniqueIndex;size:64;not null" json:"key"` // goloop_ + 32字符随机串
	Name               string     `gorm:"not null" json:"name"`
	ChannelRestriction *string    `json:"channel_restriction,omitempty"` // 限制的渠道名称，NULL 表示不限制
	Enabled            bool       `gorm:"default:true" json:"enabled"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"` // 过期时间，NULL 表示永不过期
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	TotalRequests      int64      `gorm:"default:0" json:"total_requests"`
	TotalSuccess       int64      `gorm:"default:0" json:"total_success"`
	TotalFail          int64      `gorm:"default:0" json:"total_fail"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	
	UsageLogs []UsageLog `gorm:"foreignKey:APIKeyID;constraint:OnDelete:CASCADE" json:"-"`
}

// UsageLog API Key 使用记录表（仅保留 30 天）
type UsageLog struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	APIKeyID    uint       `gorm:"not null;index:idx_apikey_created" json:"api_key_id"`
	ChannelName string     `gorm:"not null" json:"channel_name"`
	Model       string     `gorm:"not null" json:"model"`
	Success     bool       `gorm:"not null" json:"success"`
	StatusCode  *int       `json:"status_code,omitempty"`
	ErrorMessage *string   `gorm:"type:text" json:"error_message,omitempty"`
	LatencyMs   *int       `json:"latency_ms,omitempty"`
	RequestIP   *string    `json:"request_ip,omitempty"`
	ShouldCount bool       `gorm:"default:false;index:idx_should_count" json:"should_count"` // 是否计入全局统计（最终成功/失败）
	CreatedAt   time.Time  `gorm:"index:idx_created;index:idx_apikey_created" json:"created_at"`
	
	APIKey APIKey `gorm:"foreignKey:APIKeyID" json:"-"`
}
