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
	ChannelRestriction *string    `json:"channel_restriction,omitempty"` // 限制的渠道名称，NULL 表示不限制（单渠道兼容字段）
	GroupID            *uint      `gorm:"index" json:"group_id,omitempty"` // 关联的渠道分组 ID，NULL 表示不绑定分组
	Enabled            bool       `gorm:"default:true" json:"enabled"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"` // 过期时间，NULL 表示永不过期
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	TotalRequests      int64      `gorm:"default:0" json:"total_requests"`
	TotalSuccess       int64      `gorm:"default:0" json:"total_success"`
	TotalFail          int64      `gorm:"default:0" json:"total_fail"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	
	Group     *ChannelGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	UsageLogs []UsageLog    `gorm:"foreignKey:APIKeyID;constraint:OnDelete:CASCADE" json:"-"`
}

// ChannelGroup 渠道分组：一个令牌可绑定一个分组，从而约束到一组渠道
type ChannelGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 多对多：分组 <-> 渠道
	Channels []Channel `gorm:"many2many:group_channels;constraint:OnDelete:CASCADE" json:"channels,omitempty"`
}

// GroupChannel 分组-渠道关联表（由 GORM 自动维护，此处显式声明以便添加索引）
type GroupChannel struct {
	ChannelGroupID uint `gorm:"primaryKey" json:"channel_group_id"`
	ChannelID      uint `gorm:"primaryKey" json:"channel_id"`
}

// UsageLog API Key 使用记录表（仅保留 30 天）
type UsageLog struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	APIKeyID    uint       `gorm:"not null;index:idx_apikey_created" json:"api_key_id"`
	ChannelName string     `gorm:"not null" json:"channel_name"`
	Model       string     `gorm:"not null" json:"model"`
	Success     bool       `gorm:"not null" json:"success"`
	StatusCode  *int       `json:"status_code,omitempty"`
	ErrorMessage  *string   `gorm:"type:text" json:"error_message,omitempty"`
	Endpoint      *string  `json:"endpoint,omitempty"`
	UpstreamError *string  `gorm:"type:text" json:"upstream_error,omitempty"`
	LatencyMs     *int     `json:"latency_ms,omitempty"`
	RequestIP     *string  `json:"request_ip,omitempty"`
	UpstreamTTFBMs *int    `json:"upstream_ttfb_ms,omitempty"`
	BodyReadMs     *int    `json:"body_read_ms,omitempty"`
	ResponseBytes  *int    `json:"response_bytes,omitempty"`
	RetryCount     *int    `json:"retry_count,omitempty"`
	RetryMs        *int    `json:"retry_ms,omitempty"`
	ShouldCount   bool     `gorm:"default:false;index:idx_should_count" json:"should_count"` // 是否计入全局统计（最终成功/失败）
	CreatedAt   time.Time  `gorm:"index:idx_created;index:idx_apikey_created" json:"created_at"`
	
	APIKey APIKey `gorm:"foreignKey:APIKeyID" json:"-"`
}
