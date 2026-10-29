package gateway

// SetProfileReadiness binds the active-profile acceptance check during startup.
// An absent check is deliberately unready, never an implicit direct profile.
func (m *Manager) SetProfileReadiness(ready func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileReady = ready
}
