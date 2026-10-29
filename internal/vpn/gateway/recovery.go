package gateway

import "context"

// Recover restarts an unexpectedly exited proxy and restores the TUN route.
// The existing blackhole remains in place until restore has succeeded.
func (m *Manager) Recover(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile == nil || !m.enabled {
		return nil
	}
	running, err := m.profileCtrl.IsRunning(ctx)
	if err != nil {
		return err
	}
	if running && m.ready {
		return nil
	}
	m.ready = false
	return m.restoreLocked(ctx)
}
