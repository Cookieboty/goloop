package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	
	"gorm.io/gorm"
)

// Repository 数据访问层
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 Repository 实例
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ==================== Channel CRUD ====================

// GetEnabledChannels 获取所有启用的渠道
func (r *Repository) GetEnabledChannels() ([]Channel, error) {
	var channels []Channel
	err := r.db.Where("enabled = ?", true).
		Preload("Accounts", "enabled = ?", true).
		Preload("ModelMappings").
		Find(&channels).Error
	return channels, err
}

// GetAllChannels 获取所有渠道（包括禁用的）
func (r *Repository) GetAllChannels() ([]Channel, error) {
	var channels []Channel
	err := r.db.Preload("Accounts").Preload("ModelMappings").Find(&channels).Error
	return channels, err
}

// GetChannelByID 根据 ID 获取渠道
func (r *Repository) GetChannelByID(id uint) (*Channel, error) {
	var channel Channel
	err := r.db.Preload("Accounts").Preload("ModelMappings").First(&channel, id).Error
	return &channel, err
}

// CreateChannelWithAccounts 事务：创建渠道+账号+模型映射
func (r *Repository) CreateChannelWithAccounts(channel *Channel, accounts []Account, mappings []ModelMapping) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建渠道
		if err := tx.Create(channel).Error; err != nil {
			return fmt.Errorf("failed to create channel: %w", err)
		}
		
		// 2. 创建账号
		if len(accounts) > 0 {
			for i := range accounts {
				accounts[i].ChannelID = channel.ID
			}
			if err := tx.Create(&accounts).Error; err != nil {
				return fmt.Errorf("failed to create accounts: %w", err)
			}
		}
		
		// 3. 创建模型映射
		if len(mappings) > 0 {
			for i := range mappings {
				mappings[i].ChannelID = channel.ID
			}
			if err := tx.Create(&mappings).Error; err != nil {
				return fmt.Errorf("failed to create model mappings: %w", err)
			}
		}
		
		return nil
	})
}

// UpdateChannel 更新渠道
func (r *Repository) UpdateChannel(channel *Channel) error {
	return r.db.Save(channel).Error
}

// UpdateChannelWithAccounts 事务：更新渠道+重建账号+重建模型映射
func (r *Repository) UpdateChannelWithAccounts(channel *Channel, accounts []Account, mappings []ModelMapping) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 更新渠道基本信息
		if err := tx.Save(channel).Error; err != nil {
			return fmt.Errorf("failed to update channel: %w", err)
		}
		
		// 2. 删除旧账号
		if err := tx.Where("channel_id = ?", channel.ID).Delete(&Account{}).Error; err != nil {
			return fmt.Errorf("failed to delete old accounts: %w", err)
		}
		
		// 3. 创建新账号
		if len(accounts) > 0 {
			for i := range accounts {
				accounts[i].ChannelID = channel.ID
			}
			if err := tx.Create(&accounts).Error; err != nil {
				return fmt.Errorf("failed to create accounts: %w", err)
			}
		}
		
		// 4. 删除旧模型映射
		if err := tx.Where("channel_id = ?", channel.ID).Delete(&ModelMapping{}).Error; err != nil {
			return fmt.Errorf("failed to delete old model mappings: %w", err)
		}
		
		// 5. 创建新模型映射
		if len(mappings) > 0 {
			for i := range mappings {
				mappings[i].ChannelID = channel.ID
			}
			if err := tx.Create(&mappings).Error; err != nil {
				return fmt.Errorf("failed to create model mappings: %w", err)
			}
		}
		
		return nil
	})
}

// DeleteChannel 删除渠道（级联删除账号和映射）
func (r *Repository) DeleteChannel(id uint) error {
	return r.db.Delete(&Channel{}, id).Error
}

// ToggleChannel 启用/禁用渠道
func (r *Repository) ToggleChannel(id uint, enabled bool) error {
	return r.db.Model(&Channel{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// ==================== Account CRUD ====================

// GetChannelAccounts 获取渠道下的所有账号
func (r *Repository) GetChannelAccounts(channelID uint) ([]Account, error) {
	var accounts []Account
	err := r.db.Where("channel_id = ?", channelID).Find(&accounts).Error
	return accounts, err
}

// CreateAccount 创建账号
func (r *Repository) CreateAccount(account *Account) error {
	return r.db.Create(account).Error
}

// UpdateAccount 更新账号
func (r *Repository) UpdateAccount(account *Account) error {
	return r.db.Save(account).Error
}

// DeleteAccount 删除账号
func (r *Repository) DeleteAccount(id uint) error {
	return r.db.Delete(&Account{}, id).Error
}

// ToggleAccount 启用/禁用账号
func (r *Repository) ToggleAccount(id uint, enabled bool) error {
	return r.db.Model(&Account{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// ==================== ModelMapping CRUD ====================

// GetModelMappings 获取渠道的模型映射
func (r *Repository) GetModelMappings(channelID uint) ([]ModelMapping, error) {
	var mappings []ModelMapping
	err := r.db.Where("channel_id = ?", channelID).Find(&mappings).Error
	return mappings, err
}

// CreateModelMapping 创建模型映射
func (r *Repository) CreateModelMapping(mapping *ModelMapping) error {
	return r.db.Create(mapping).Error
}

// UpdateModelMapping 更新模型映射
func (r *Repository) UpdateModelMapping(mapping *ModelMapping) error {
	return r.db.Save(mapping).Error
}

// DeleteModelMapping 删除模型映射
func (r *Repository) DeleteModelMapping(id uint) error {
	return r.db.Delete(&ModelMapping{}, id).Error
}

// ==================== APIKey CRUD ====================

// GenerateAPIKey 生成 API Key (goloop_ + 32字符随机串)
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "goloop_" + hex.EncodeToString(bytes), nil
}

// CreateAPIKey 创建 API Key
func (r *Repository) CreateAPIKey(apiKey *APIKey) error {
	if apiKey.Key == "" {
		key, err := GenerateAPIKey()
		if err != nil {
			return fmt.Errorf("failed to generate api key: %w", err)
		}
		apiKey.Key = key
	}
	return r.db.Create(apiKey).Error
}

// GetAPIKeyByKey 根据 key 字符串获取 API Key
func (r *Repository) GetAPIKeyByKey(key string) (*APIKey, error) {
	var apiKey APIKey
	err := r.db.Where("key = ?", key).First(&apiKey).Error
	return &apiKey, err
}

// GetAllAPIKeys 获取所有 API Key
func (r *Repository) GetAllAPIKeys() ([]APIKey, error) {
	var apiKeys []APIKey
	err := r.db.Find(&apiKeys).Error
	return apiKeys, err
}

// GetAPIKeyByID 根据 ID 获取 API Key
func (r *Repository) GetAPIKeyByID(id uint) (*APIKey, error) {
	var apiKey APIKey
	err := r.db.First(&apiKey, id).Error
	return &apiKey, err
}

// UpdateAPIKey 更新 API Key
func (r *Repository) UpdateAPIKey(apiKey *APIKey) error {
	return r.db.Save(apiKey).Error
}

// DeleteAPIKey 删除 API Key
func (r *Repository) DeleteAPIKey(id uint) error {
	return r.db.Delete(&APIKey{}, id).Error
}

// ToggleAPIKey 启用/禁用 API Key
func (r *Repository) ToggleAPIKey(id uint, enabled bool) error {
	return r.db.Model(&APIKey{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// ==================== UsageLog 批量操作 ====================

// LogEntry 使用日志条目（用于批量插入）
type LogEntry struct {
	APIKeyID     uint
	ChannelName  string
	Model        string
	Success      bool
	StatusCode   *int
	ErrorMessage *string
	LatencyMs    *int
	RequestIP    *string
	UpdateStats  bool // 是否更新 API Key 的统计数据（TotalSuccess/TotalFail）
}

// BatchInsertUsageLogs 批量插入使用日志
func (r *Repository) BatchInsertUsageLogs(entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	
	logs := make([]UsageLog, len(entries))
	for i, entry := range entries {
		logs[i] = UsageLog{
			APIKeyID:     entry.APIKeyID,
			ChannelName:  entry.ChannelName,
			Model:        entry.Model,
			Success:      entry.Success,
			StatusCode:   entry.StatusCode,
			ErrorMessage: entry.ErrorMessage,
			LatencyMs:    entry.LatencyMs,
			RequestIP:    entry.RequestIP,
			CreatedAt:    time.Now(),
		}
	}
	
	return r.db.Create(&logs).Error
}

// UpdateAPIKeyStats 原子更新 API Key 统计字段
// 只统计 UpdateStats=true 的记录到 TotalSuccess/TotalFail
func (r *Repository) UpdateAPIKeyStats(entries []LogEntry) error {
	// 按 APIKeyID 聚合统计（只统计 UpdateStats=true 的记录）
	stats := make(map[uint]struct{ success, fail int64 })
	for _, entry := range entries {
		if !entry.UpdateStats {
			continue // 跳过不需要更新统计的记录
		}
		s := stats[entry.APIKeyID]
		if entry.Success {
			s.success++
		} else {
			s.fail++
		}
		stats[entry.APIKeyID] = s
	}
	
	// 批量更新
	now := time.Now()
	for apiKeyID, stat := range stats {
		err := r.db.Model(&APIKey{}).Where("id = ?", apiKeyID).Updates(map[string]interface{}{
			"total_requests": gorm.Expr("total_requests + ?", stat.success+stat.fail),
			"total_success":  gorm.Expr("total_success + ?", stat.success),
			"total_fail":     gorm.Expr("total_fail + ?", stat.fail),
			"last_used_at":   now,
		}).Error
		if err != nil {
			return err
		}
	}
	
	return nil
}

// GetUsageLogs 获取 API Key 的使用记录（分页）
func (r *Repository) GetUsageLogs(apiKeyID uint, limit, offset int, startDate, endDate *time.Time) ([]UsageLog, error) {
	query := r.db.Where("api_key_id = ?", apiKeyID)
	
	if startDate != nil {
		query = query.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", *endDate)
	}
	
	var logs []UsageLog
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, err
}

// GetErrorLogs 获取错误日志（只返回失败的记录）
func (r *Repository) GetErrorLogs(limit, offset int, startDate, endDate *time.Time) ([]UsageLog, error) {
	query := r.db.Where("success = ?", false)
	
	if startDate != nil {
		query = query.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", *endDate)
	}
	
	var logs []UsageLog
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, err
}

// GetErrorLogsCount 获取错误日志总数（用于分页）
func (r *Repository) GetErrorLogsCount(startDate, endDate *time.Time) (int64, error) {
	query := r.db.Model(&UsageLog{}).Where("success = ?", false)
	
	if startDate != nil {
		query = query.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", *endDate)
	}
	
	var count int64
	err := query.Count(&count).Error
	return count, err
}

// DeleteUsageLogsBefore 删除指定时间之前的日志
func (r *Repository) DeleteUsageLogsBefore(cutoffTime time.Time) (int64, error) {
	result := r.db.Where("created_at < ?", cutoffTime).Delete(&UsageLog{})
	return result.RowsAffected, result.Error
}

// GetAPIKeyStatsByChannel 获取 API Key 按渠道分组的统计
func (r *Repository) GetAPIKeyStatsByChannel(apiKeyID uint, startDate, endDate *time.Time) (map[string]struct {
	TotalRequests int64
	TotalSuccess  int64
	TotalFail     int64
	AvgLatencyMs  float64
}, error) {
	type result struct {
		ChannelName   string
		TotalRequests int64
		TotalSuccess  int64
		TotalFail     int64
		AvgLatencyMs  float64
	}
	
	query := r.db.Model(&UsageLog{}).Where("api_key_id = ?", apiKeyID)
	if startDate != nil {
		query = query.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		query = query.Where("created_at <= ?", *endDate)
	}
	
	var results []result
	err := query.Select(
		"channel_name",
		"COUNT(*) as total_requests",
		"SUM(CASE WHEN success THEN 1 ELSE 0 END) as total_success",
		"SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as total_fail",
		"AVG(latency_ms) as avg_latency_ms",
	).Group("channel_name").Find(&results).Error
	
	if err != nil {
		return nil, err
	}
	
	stats := make(map[string]struct {
		TotalRequests int64
		TotalSuccess  int64
		TotalFail     int64
		AvgLatencyMs  float64
	})
	
	for _, r := range results {
		stats[r.ChannelName] = struct {
			TotalRequests int64
			TotalSuccess  int64
			TotalFail     int64
			AvgLatencyMs  float64
		}{
			TotalRequests: r.TotalRequests,
			TotalSuccess:  r.TotalSuccess,
			TotalFail:     r.TotalFail,
			AvgLatencyMs:  r.AvgLatencyMs,
		}
	}
	
	return stats, nil
}

// ==================== 全局统计 ====================

// OverviewStats 全局概览统计
type OverviewStats struct {
	TotalRequests int64   `json:"total_requests"`
	TotalSuccess  int64   `json:"total_success"`
	TotalFail     int64   `json:"total_fail"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// ChannelTypeStats 渠道类型汇总统计
type ChannelTypeStats struct {
	Gemini OverviewStats `json:"gemini"`
	OpenAI OverviewStats `json:"openai"`
	Today  OverviewStats `json:"today"`
}

// ChannelDetailStats 单个渠道详细统计
type ChannelDetailStats struct {
	ChannelName   string  `json:"channel_name"`
	ChannelType   string  `json:"channel_type"`
	TotalRequests int64   `json:"total_requests"`
	TotalSuccess  int64   `json:"total_success"`
	TotalFail     int64   `json:"total_fail"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// GetGlobalStats 获取全局统计数据（Gemini/OpenAI汇总 + 今日统计 + 各渠道详情）
func (r *Repository) GetGlobalStats() (*ChannelTypeStats, []ChannelDetailStats, error) {
	// 1. 查询每个渠道的统计数据
	// JOIN channels 表以确保只统计当前存在的渠道，避免历史数据重复
	type channelResult struct {
		ChannelName   string
		ChannelType   string
		TotalRequests int64
		TotalSuccess  int64
		TotalFail     int64
		AvgLatencyMs  float64
	}
	
	var channelResults []channelResult
	err := r.db.Model(&UsageLog{}).
		Joins("INNER JOIN channels ON usage_logs.channel_name = channels.name").
		Where("channels.enabled = ?", true).
		Select(
			"usage_logs.channel_name",
			"channels.type as channel_type",
			"COUNT(*) as total_requests",
			"SUM(CASE WHEN success THEN 1 ELSE 0 END) as total_success",
			"SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as total_fail",
			"COALESCE(AVG(usage_logs.latency_ms), 0) as avg_latency_ms",
		).
		Group("usage_logs.channel_name, channels.type").
		Find(&channelResults).Error
	
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query channel stats: %w", err)
	}
	
	// 2. 查询今日统计
	today := time.Now().Truncate(24 * time.Hour)
	var todayResult struct {
		TotalRequests int64
		TotalSuccess  int64
		TotalFail     int64
		AvgLatencyMs  float64
	}
	
	err = r.db.Model(&UsageLog{}).
		Where("created_at >= ?", today).
		Select(
			"COUNT(*) as total_requests",
			"SUM(CASE WHEN success THEN 1 ELSE 0 END) as total_success",
			"SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as total_fail",
			"COALESCE(AVG(latency_ms), 0) as avg_latency_ms",
		).
		Scan(&todayResult).Error
	
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query today stats: %w", err)
	}
	
	// 3. 汇总 Gemini 和 OpenAI 统计
	var geminiStats, openaiStats OverviewStats
	channelDetails := make([]ChannelDetailStats, 0)
	
	for _, cr := range channelResults {
		// channelType 已经从 JOIN 查询中获取，无需再查找
		
		// 计算成功率
		successRate := 0.0
		if cr.TotalRequests > 0 {
			successRate = float64(cr.TotalSuccess) / float64(cr.TotalRequests) * 100
		}
		
		// 添加到渠道详情列表
		channelDetails = append(channelDetails, ChannelDetailStats{
			ChannelName:   cr.ChannelName,
			ChannelType:   cr.ChannelType,
			TotalRequests: cr.TotalRequests,
			TotalSuccess:  cr.TotalSuccess,
			TotalFail:     cr.TotalFail,
			SuccessRate:   successRate,
			AvgLatencyMs:  cr.AvgLatencyMs,
		})
		
		// 按类型汇总（检查渠道类型是否包含 "gemini" 或 "openai"）
		isGemini := false
		isOpenAI := false
		
		// 简单的类型判断逻辑
		switch cr.ChannelType {
		case "gemini_callback", "gemini_openai", "gemini_original":
			isGemini = true
		case "openai_original", "openai_callback":
			isOpenAI = true
		}
		
		if isGemini {
			geminiStats.TotalRequests += cr.TotalRequests
			geminiStats.TotalSuccess += cr.TotalSuccess
			geminiStats.TotalFail += cr.TotalFail
			// 延迟需要加权平均，这里简化为累加（后面会重新计算）
		} else if isOpenAI {
			openaiStats.TotalRequests += cr.TotalRequests
			openaiStats.TotalSuccess += cr.TotalSuccess
			openaiStats.TotalFail += cr.TotalFail
		}
	}
	
	// 4. 重新计算 Gemini 和 OpenAI 的平均延迟
	geminiStats.AvgLatencyMs = r.calculateAvgLatency("gemini")
	openaiStats.AvgLatencyMs = r.calculateAvgLatency("openai")
	
	typeStats := &ChannelTypeStats{
		Gemini: geminiStats,
		OpenAI: openaiStats,
		Today: OverviewStats{
			TotalRequests: todayResult.TotalRequests,
			TotalSuccess:  todayResult.TotalSuccess,
			TotalFail:     todayResult.TotalFail,
			AvgLatencyMs:  todayResult.AvgLatencyMs,
		},
	}
	
	return typeStats, channelDetails, nil
}

// calculateAvgLatency 计算指定类型渠道的平均延迟
func (r *Repository) calculateAvgLatency(typePrefix string) float64 {
	// 查找匹配的渠道
	channels, err := r.GetAllChannels()
	if err != nil {
		return 0
	}
	
	var channelNames []string
	for _, ch := range channels {
		if typePrefix == "gemini" {
			if ch.Type == "gemini_callback" || ch.Type == "gemini_openai" || ch.Type == "gemini_original" {
				channelNames = append(channelNames, ch.Name)
			}
		} else if typePrefix == "openai" {
			if ch.Type == "openai_original" || ch.Type == "openai_callback" {
				channelNames = append(channelNames, ch.Name)
			}
		}
	}
	
	if len(channelNames) == 0 {
		return 0
	}
	
	var avgLatency float64
	err = r.db.Model(&UsageLog{}).
		Where("channel_name IN ?", channelNames).
		Select("COALESCE(AVG(latency_ms), 0)").
		Scan(&avgLatency).Error
	
	if err != nil {
		return 0
	}
	
	return avgLatency
}
