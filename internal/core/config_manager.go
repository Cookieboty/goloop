package core

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
	
	"goloop/internal/database"
)

// ChannelConfig 渠道配置（内存）
type ChannelConfig struct {
	ID                    uint
	Name                  string
	Type                  string
	BaseURL               string
	Weight                int
	Timeout               time.Duration
	InitialInterval       time.Duration
	MaxInterval           time.Duration
	MaxWaitTime           time.Duration
	RetryAttempts         int
	ProbeModel            string
	Accounts              []AccountConfig
}

// AccountConfig 账号配置（内存）
type AccountConfig struct {
	APIKey string
	Weight int
}

// ConfigManager 配置管理器（内存缓存 + 热更新）
type ConfigManager struct {
	mu            sync.RWMutex
	channels      map[string]*ChannelConfig       // name -> config
	modelMappings map[string]map[string]string    // channel_name -> (source_model -> target_model)
	repo          *database.Repository
}

// NewConfigManager 创建配置管理器
func NewConfigManager(repo *database.Repository) *ConfigManager {
	return &ConfigManager{
		channels:      make(map[string]*ChannelConfig),
		modelMappings: make(map[string]map[string]string),
		repo:          repo,
	}
}

// Load 从数据库加载配置到内存
func (m *ConfigManager) Load() error {
	channels, err := m.repo.GetEnabledChannels()
	if err != nil {
		return fmt.Errorf("failed to load channels: %w", err)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	newChannels := make(map[string]*ChannelConfig)
	newMappings := make(map[string]map[string]string)
	
	for _, ch := range channels {
		// 转换账号
		accounts := make([]AccountConfig, 0, len(ch.Accounts))
		for _, acc := range ch.Accounts {
			accounts = append(accounts, AccountConfig{
				APIKey: acc.APIKey,
				Weight: acc.Weight,
			})
		}
		
		// 转换模型映射
		mappingMap := make(map[string]string)
		for _, mapping := range ch.ModelMappings {
			mappingMap[mapping.SourceModel] = mapping.TargetModel
		}
		
		// 创建配置
		cfg := &ChannelConfig{
			ID:       ch.ID,
			Name:     ch.Name,
			Type:     ch.Type,
			BaseURL:  ch.BaseURL,
			Weight:   ch.Weight,
			Timeout:  time.Duration(ch.TimeoutSeconds) * time.Second,
			Accounts: accounts,
		}
		
		// 可选字段
		if ch.InitialIntervalSeconds != nil {
			cfg.InitialInterval = time.Duration(*ch.InitialIntervalSeconds) * time.Second
		}
		if ch.MaxIntervalSeconds != nil {
			cfg.MaxInterval = time.Duration(*ch.MaxIntervalSeconds) * time.Second
		}
		if ch.MaxWaitTimeSeconds != nil {
			cfg.MaxWaitTime = time.Duration(*ch.MaxWaitTimeSeconds) * time.Second
		}
		if ch.RetryAttempts != nil {
			cfg.RetryAttempts = *ch.RetryAttempts
		}
		if ch.ProbeModel != nil {
			cfg.ProbeModel = *ch.ProbeModel
		}
		
		// Log channel config for debugging
		slog.Debug("loaded channel config",
			"name", ch.Name,
			"type", ch.Type,
			"timeout", cfg.Timeout,
			"maxWaitTime", cfg.MaxWaitTime,
			"initialInterval", cfg.InitialInterval,
			"maxInterval", cfg.MaxInterval,
		)
		
		newChannels[ch.Name] = cfg
		newMappings[ch.Name] = mappingMap
	}
	
	m.channels = newChannels
	m.modelMappings = newMappings
	
	slog.Info("config loaded", "channels", len(newChannels))
	return nil
}

// Reload 重新加载配置（热更新）
func (m *ConfigManager) Reload() error {
	return m.Load()
}

// GetChannel 获取渠道配置（只读）
func (m *ConfigManager) GetChannel(name string) (*ChannelConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

// GetAllChannels 获取所有渠道配置（只读）
func (m *ConfigManager) GetAllChannels() map[string]*ChannelConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 返回副本，避免外部修改
	result := make(map[string]*ChannelConfig, len(m.channels))
	for k, v := range m.channels {
		result[k] = v
	}
	return result
}

// GetModelMapping 获取模型映射（只读）
func (m *ConfigManager) GetModelMapping(channelName, sourceModel string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if mappings, ok := m.modelMappings[channelName]; ok {
		if targetModel, ok := mappings[sourceModel]; ok {
			return targetModel
		}
	}
	return sourceModel // 没有映射，返回原模型
}
