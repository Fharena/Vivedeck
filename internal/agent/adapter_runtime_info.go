package agent

type AdapterRuntimeInfo struct {
	Name          string              `json:"name"`
	Mode          string              `json:"mode,omitempty"`
	Ready         bool                `json:"ready"`
	Capabilities  AdapterCapabilities `json:"capabilities"`
	WorkspaceRoot string              `json:"workspaceRoot,omitempty"`
	Binary        string              `json:"binary,omitempty"`
	BinaryPath    string              `json:"binaryPath,omitempty"`
	BridgeAddress string              `json:"bridgeAddress,omitempty"`
	TempRoot      string              `json:"tempRoot,omitempty"`
	PromptTimeout string              `json:"promptTimeout,omitempty"`
	RunTimeout    string              `json:"runTimeout,omitempty"`
	Notes         []string            `json:"notes,omitempty"`
}

type AdapterRuntimeInfoProvider interface {
	RuntimeInfo() AdapterRuntimeInfo
}

func BasicAdapterRuntimeInfo(adapter WorkspaceAdapter) AdapterRuntimeInfo {
	if adapter == nil {
		return AdapterRuntimeInfo{Ready: false}
	}

	return AdapterRuntimeInfo{
		Name:         adapter.Name(),
		Ready:        true,
		Capabilities: adapter.Capabilities(),
	}
}
