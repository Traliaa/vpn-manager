package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type configSnapshot struct {
	data      []byte
	existed   bool
	outbounds []Outbound
	template  map[string]any
}

func cloneTemplate(base map[string]any) map[string]any {
	next := make(map[string]any, len(base))
	for k, v := range base {
		next[k] = v
	}
	return next
}

func (c *Controller) updateLocked(ctx context.Context, outbounds []Outbound, base map[string]any) error {
	data, err := os.ReadFile(c.configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	old := &configSnapshot{data: data, existed: err == nil, outbounds: append([]Outbound(nil), c.activeOutbounds...), template: cloneTemplate(c.baseTemplate)}
	var built []map[string]any
	tags := map[string]bool{"direct": true, "block": true}
	for _, ob := range outbounds {
		if ob == nil || ob.Tag() == "" {
			return fmt.Errorf("outbound must have a tag")
		}
		if tags[ob.Tag()] {
			return fmt.Errorf("duplicate or reserved outbound tag")
		}
		tags[ob.Tag()] = true
		value, err := ob.BuildOutbound()
		if err != nil {
			return fmt.Errorf("build outbound: %w", err)
		}
		if value["tag"] != ob.Tag() {
			return fmt.Errorf("outbound tag mismatch")
		}
		built = append(built, value)
	}
	built = append(built, map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "block", "tag": "block"})
	cfg := cloneTemplate(base)
	cfg["outbounds"] = built
	c.ensureDefaults(cfg)
	if _, ok := cfg["route"]; !ok {
		cfg["route"] = map[string]any{"final": "direct", "auto_detect_interface": true}
	}
	if _, ok := cfg["inbounds"]; !ok {
		cfg["inbounds"] = []map[string]any{{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 1080}}
	}
	// Independently check every route target: silently skipped or missing VPN
	// targets would turn a requested VPN rule into a direct connection.
	raw, err := json.Marshal(cfg["route"])
	if err != nil {
		return err
	}
	var routes struct {
		Final string `json:"final"`
		Rules []struct {
			Outbound string `json:"outbound"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &routes); err != nil {
		return err
	}
	if routes.Final != "" && !tags[routes.Final] {
		return fmt.Errorf("default outbound is not registered")
	}
	for _, r := range routes.Rules {
		if r.Outbound != "" && !tags[r.Outbound] {
			return fmt.Errorf("route references an unregistered outbound")
		}
	}
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.configPath), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.configPath), ".candidate-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := c.validatePath(ctx, name); err != nil {
		return err
	}
	if err := os.Rename(name, c.configPath); err != nil {
		return err
	}
	c.activeOutbounds = append([]Outbound(nil), outbounds...)
	c.baseTemplate = cloneTemplate(base)
	if c.rollback == nil {
		c.rollback = old
	}
	return nil
}

func (c *Controller) restoreLocked(old *configSnapshot) error {
	if old.existed {
		tmp, err := os.CreateTemp(filepath.Dir(c.configPath), ".restore-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(old.data); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), c.configPath); err != nil {
			return err
		}
	} else if err := os.Remove(c.configPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	c.activeOutbounds = append([]Outbound(nil), old.outbounds...)
	c.baseTemplate = cloneTemplate(old.template)
	return nil
}
