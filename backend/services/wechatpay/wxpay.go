package wechatpay

import "sync"

var (
	globalClient Client
	globalCfg    *Config
	initOnce     sync.Once
)

func NewClient(cfg *Config) Client {
	if cfg.MockMode {
		return newMockClient(cfg)
	}
	return newRealClient(cfg)
}

func InitGlobal(cfg *Config) {
	initOnce.Do(func() {
		globalCfg = cfg
		globalClient = NewClient(cfg)
	})
}

func GetClient() Client {
	return globalClient
}

func GetConfig() *Config {
	return globalCfg
}
