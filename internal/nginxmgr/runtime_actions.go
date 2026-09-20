package nginxmgr

import (
	"context"
	"fmt"
)

type ReloadResult struct {
	TestOutput   string `json:"test_output"`
	ReloadOutput string `json:"reload_output"`
}

func (m *Manager) SafeReload(ctx context.Context) (ReloadResult, error) {
	testOutput, err := m.Runner.Run(ctx, m.Runtime.Path, "-t")
	if err != nil {
		return ReloadResult{TestOutput: testOutput}, fmt.Errorf("nginx -t failed; reload was not attempted: %s", testOutput)
	}

	reloadOutput, err := m.Runner.Run(ctx, m.Runtime.Path, "-s", "reload")
	if err != nil {
		return ReloadResult{
			TestOutput:   testOutput,
			ReloadOutput: reloadOutput,
		}, fmt.Errorf("nginx reload failed: %s", reloadOutput)
	}
	return ReloadResult{TestOutput: testOutput, ReloadOutput: reloadOutput}, nil
}
