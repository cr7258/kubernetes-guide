package filter1

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

type HttpPluginContext struct {
	types.DefaultPluginContext
}

// OnPluginStart 从 envoy 的配置文件中 config.configuration 获取配置
func (*HttpPluginContext) OnPluginStart(int) types.OnPluginStartStatus {
	cfg, err := proxywasm.GetPluginConfiguration()
	if err != nil {
		proxywasm.LogError(err.Error())
	} else {
		proxywasm.LogInfo(string(cfg))
	}
	return types.OnPluginStartStatusOK
}

func (*HttpPluginContext) NewHttpContext(uint32) types.HttpContext {
	return &MyHttpContext{}
}
