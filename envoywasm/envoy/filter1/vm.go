package filter1

import "github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"

type MyVM struct {
	types.DefaultVMContext
}

func (*MyVM) NewPluginContext(contextID uint32) types.PluginContext {
	return &HttpPluginContext{}
}
