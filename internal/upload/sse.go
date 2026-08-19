package upload

import (
	"context"
	"encoding/json"
)

func (m *Manager) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	m.subMu.Lock()
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()
	return ch
}

func (m *Manager) Unsubscribe(ch chan []byte) {
	m.subMu.Lock()
	delete(m.subs, ch)
	m.subMu.Unlock()
	close(ch)
}

func (m *Manager) SnapshotPayload() []byte {
	tasks := m.List(context.Background(), 0)
	payload, _ := json.Marshal(map[string]any{"tasks": tasks})
	return payload
}

func (m *Manager) broadcast() {
	payload := m.SnapshotPayload()
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}
