package singbox

import (
	"context"
	"encoding/json"
	"fmt"
)

// SetGateway enables the manually routed transparent input. It never installs
// host routes: the gateway manager owns source-specific routing separately.
func (c *Controller) SetGateway(ctx context.Context, enabled bool) error {
	c.mu.Lock()
	next := cloneTemplate(c.baseTemplate)
	var inputs []map[string]any
	if raw, ok := next["inbounds"]; ok {
		data, err := json.Marshal(raw)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		if err := json.Unmarshal(data, &inputs); err != nil {
			c.mu.Unlock()
			return err
		}
	} else {
		inputs = []map[string]any{{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 1080}}
	}
	kept := make([]map[string]any, 0, len(inputs)+1)
	for _, input := range inputs {
		if input["tag"] == "gateway-in" {
			continue
		}
		if enabled && input["interface_name"] == "vmtun0" {
			c.mu.Unlock()
			return fmt.Errorf("gateway interface is already used by another input")
		}
		kept = append(kept, input)
	}
	if enabled {
		kept = append(kept, map[string]any{"type": "tun", "tag": "gateway-in", "interface_name": "vmtun0", "address": []string{"172.31.255.1/30"}, "mtu": 1400, "auto_route": false, "stack": "gvisor", "sniff": true, "sniff_override_destination": false})
	}
	next["inbounds"] = kept
	err := c.updateLocked(ctx, c.activeOutbounds, next)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	defer c.mu.Unlock()
	return c.reloadLocked(ctx)
}
