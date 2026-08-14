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

// ResetGlobalForTesting clears the singleton so a test can re-initialize
// with a different config (e.g. non-mock) — production code never calls it.
func ResetGlobalForTesting() {
	initOnce = sync.Once{}
	globalCfg = nil
	globalClient = nil
}

// SetClientForTesting replaces the global client with a stub (handlers-package
// tests use this to drive non-mock branches like membership JSAPI without
// real WeChat credentials).
func SetClientForTesting(c Client, cfg *Config) {
	globalClient = c
	globalCfg = cfg
}

func GetClient() Client {
	return globalClient
}

func GetConfig() *Config {
	return globalCfg
}
