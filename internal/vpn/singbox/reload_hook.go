package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// SetReloadHook installs the gateway's route reconciler. The callback runs while
// the controller lock is held and must not call back into this controller.
func (c *Controller) SetReloadHook(hook func(context.Context, bool) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadHook = hook
}
func (c *Controller) startRuntimeLocked(ctx context.Context) error {
	if err := c.process.Start(ctx, c.binaryPath, []string{"run", "-c", c.configPath}, nil); err != nil {
		return err
	}
	if c.reloadHook == nil {
		return nil
	}
	data, err := json.Marshal(c.baseTemplate["inbounds"])
	if err != nil {
		return err
	}
	var inputs []struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
	}
	if err = json.Unmarshal(data, &inputs); err != nil {
		return err
	}
	active := false
	for _, input := range inputs {
		if input.Type == "tun" && input.Tag == "gateway-in" {
			active = true
		}
	}
	if err = c.reloadHook(ctx, active); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return errors.Join(err, c.process.Stop(cleanup))
	}
	return nil
}
