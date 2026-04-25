package main

import (
	"log/slog"
	"time"

	"goloop/internal/channels/gemini_callback"
	"goloop/internal/channels/gemini_openai"
	"goloop/internal/channels/gemini_original"
	"goloop/internal/channels/openai_callback"
	"goloop/internal/channels/openai_original"
	"goloop/internal/core"
	"goloop/internal/storage"
)

// ChannelBootstrapper 负责从配置创建和注册通道
type ChannelBootstrapper struct {
	registry  *core.PluginRegistry
	configMgr *core.ConfigManager
	store     *storage.Store
}

// NewChannelBootstrapper 创建通道引导服务
func NewChannelBootstrapper(registry *core.PluginRegistry, configMgr *core.ConfigManager, store *storage.Store) *ChannelBootstrapper {
	return &ChannelBootstrapper{
		registry:  registry,
		configMgr: configMgr,
		store:     store,
	}
}

// Bootstrap 从配置初始化并注册所有通道
func (b *ChannelBootstrapper) Bootstrap() error {
	for name, chCfg := range b.configMgr.GetAllChannels() {
		b.registerChannel(name, chCfg)
	}
	
	if len(b.registry.List()) == 0 {
		slog.Warn("no channels registered, running in degraded mode")
	}
	
	return nil
}

// ReloadAndRegister 重新加载配置并重新注册所有通道
func (b *ChannelBootstrapper) ReloadAndRegister() error {
	// 1. 重新加载配置
	if err := b.configMgr.Reload(); err != nil {
		return err
	}
	
	// 2. 清理旧通道
	b.registry.Clear()
	
	// 3. 重新注册所有通道
	for name, chCfg := range b.configMgr.GetAllChannels() {
		b.registerChannel(name, chCfg)
	}
	
	slog.Info("channels reloaded and re-registered", "count", len(b.registry.List()))
	return nil
}

// registerChannel 注册单个通道
func (b *ChannelBootstrapper) registerChannel(name string, chCfg *core.ChannelConfig) {
	switch chCfg.Type {
	case "gemini_callback":
		pool := gemini_callback.NewAccountPool()
		for _, acc := range chCfg.Accounts {
			pool.AddAccount(acc.APIKey, acc.Weight)
		}
		timeout := chCfg.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		
		slog.Debug("creating gemini_callback channel",
			"name", name,
			"timeout", timeout,
			"initialInterval", chCfg.InitialInterval,
			"maxInterval", chCfg.MaxInterval,
			"maxWaitTime", chCfg.MaxWaitTime,
			"retryAttempts", chCfg.RetryAttempts,
		)
		
		kieCh := gemini_callback.NewChannel(name, chCfg.BaseURL, chCfg.Weight, pool, gemini_callback.Config{
			BaseURL:         chCfg.BaseURL,
			Timeout:         timeout,
			InitialInterval: chCfg.InitialInterval,
			MaxInterval:     chCfg.MaxInterval,
			MaxWaitTime:     chCfg.MaxWaitTime,
			RetryAttempts:   chCfg.RetryAttempts,
		}, b.store)
		b.registry.Register(kieCh)
		slog.Info("channel registered", "name", name, "type", chCfg.Type, "accounts", len(chCfg.Accounts))

	case "gemini_openai":
		pool := core.NewDefaultAccountPool()
		for _, acc := range chCfg.Accounts {
			pool.AddAccount(acc.APIKey, acc.Weight)
		}
		timeout := chCfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		probeModel := chCfg.ProbeModel
		if probeModel == "" {
			probeModel = "gpt-4o-mini"
		}
		subCh := gemini_openai.NewChannel(name, chCfg.BaseURL, chCfg.Weight, pool, timeout, gemini_openai.Config{
			ProbeModel: probeModel,
		})
		b.registry.Register(subCh)
		slog.Info("channel registered", "name", name, "type", chCfg.Type, "accounts", len(chCfg.Accounts))

	case "gemini_original":
		pool := core.NewDefaultAccountPool()
		for _, acc := range chCfg.Accounts {
			pool.AddAccount(acc.APIKey, acc.Weight)
		}
		timeout := chCfg.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		gemCh := gemini_original.NewChannel(name, chCfg.BaseURL, chCfg.Weight, pool, timeout)
		b.registry.Register(gemCh)
		slog.Info("channel registered", "name", name, "type", chCfg.Type, "accounts", len(chCfg.Accounts))

	case "openai_original":
		pool := core.NewDefaultAccountPool()
		for _, acc := range chCfg.Accounts {
			pool.AddAccount(acc.APIKey, acc.Weight)
		}
		timeout := chCfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		gptImageCh := openai_original.NewChannel(name, chCfg.BaseURL, chCfg.Weight, pool, timeout)
		b.registry.Register(gptImageCh)
		slog.Info("channel registered", "name", name, "type", chCfg.Type, "accounts", len(chCfg.Accounts))

	case "openai_callback":
		pool := openai_callback.NewAccountPool()
		for _, acc := range chCfg.Accounts {
			pool.AddAccount(acc.APIKey, acc.Weight)
		}
		timeout := chCfg.Timeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		openaiCh := openai_callback.NewChannel(name, chCfg.BaseURL, chCfg.Weight, pool, openai_callback.Config{
			BaseURL:         chCfg.BaseURL,
			Timeout:         timeout,
			InitialInterval: chCfg.InitialInterval,
			MaxInterval:     chCfg.MaxInterval,
			MaxWaitTime:     chCfg.MaxWaitTime,
			RetryAttempts:   chCfg.RetryAttempts,
		})
		b.registry.Register(openaiCh)
		slog.Info("channel registered", "name", name, "type", chCfg.Type, "accounts", len(chCfg.Accounts))

	default:
		slog.Warn("unknown channel type, skipping", "name", name, "type", chCfg.Type)
	}
}
