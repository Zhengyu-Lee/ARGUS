package device

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Manager 是设备状态管理器接口
type Manager interface {
	// Acquire 分配一台空闲设备给指定平台
	Acquire(platform string, taskID string) (*Device, error)
	// Release 释放设备
	Release(deviceID string) error
	// UpdateStatus 更新设备状态
	UpdateStatus(deviceID string, status string) error
	// Heartbeat 设备心跳
	Heartbeat(deviceID string, healthScore int) error
	// List 列出所有设备
	List() ([]Device, error)
	// Get 获取单个设备
	Get(deviceID string) (*Device, error)
	// UpdateLoginStatus 更新登录态
	UpdateLoginStatus(deviceID string, platform string, loggedIn bool, accountName string) error
	// Register 注册新设备
	Register(device Device) error
	// Stats 获取设备池统计
	Stats() PoolStats
	// Close
	Close() error
}

type PoolStats struct {
	Total       int            `json:"total"`
	Idle        int            `json:"idle"`
	Busy        int            `json:"busy"`
	Offline     int            `json:"offline"`
	Maintenance int            `json:"maintenance"`
	Banned      int            `json:"banned"`
	ByPlatform  map[string]int `json:"by_platform"`
	AvgHealth   int            `json:"avg_health"`
}

// ── Memory Manager (PoC / 单实例) ──

type MemoryManager struct {
	mu      sync.RWMutex
	devices map[string]*Device
}

func NewMemoryManager() *MemoryManager {
	m := &MemoryManager{
		devices: make(map[string]*Device),
	}
	// 注册一些模拟设备用于 PoC
	for i := 1; i <= 5; i++ {
		m.devices[fmt.Sprintf("device-%d", i)] = &Device{
			ID:        fmt.Sprintf("device-%d", i),
			Name:      fmt.Sprintf("Android-0%d", i),
			ADBSerial: fmt.Sprintf("emulator-%d", 5554+i*2),
			Status:    StatusIdle,
			HealthScore: 95,
			Tags:      []string{"poc"},
			LoginStatuses: []PlatformLoginStatus{
				{Platform: PlatformDouyin, LoggedIn: true, AccountName: fmt.Sprintf("test_dy_%d", i)},
				{Platform: PlatformKuaishou, LoggedIn: true, AccountName: fmt.Sprintf("test_ks_%d", i)},
			},
		}
	}
	return m
}

func (m *MemoryManager) Acquire(platform string, taskID string) (*Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 找一台空闲的、有该平台登录态的设备
	for _, d := range m.devices {
		if d.Status != StatusIdle {
			continue
		}
		// 检查该平台是否有登录态
		hasLogin := false
		for _, ls := range d.LoginStatuses {
			if ls.Platform == platform && ls.LoggedIn {
				hasLogin = true
				break
			}
		}
		if !hasLogin {
			continue
		}

		d.Status = StatusBusy
		d.CurrentTask = taskID
		d.AssignedPlatform = platform
		return d, nil
	}

	return nil, fmt.Errorf("no available device for platform %s (idle=%d, total=%d)",
		platform, m.countByStatus(StatusIdle), len(m.devices))
}

func (m *MemoryManager) Release(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}
	d.Status = StatusIdle
	d.CurrentTask = ""
	d.AssignedPlatform = ""
	d.HealthScore = min(100, d.HealthScore+5) // 恢复健康分
	return nil
}

func (m *MemoryManager) UpdateStatus(deviceID string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}
	d.Status = status
	return nil
}

func (m *MemoryManager) Heartbeat(deviceID string, healthScore int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}
	d.LastHeartbeat = time.Now()
	if healthScore > 0 {
		d.HealthScore = healthScore
	}
	return nil
}

func (m *MemoryManager) List() ([]Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		devices = append(devices, *d)
	}
	return devices, nil
}

func (m *MemoryManager) Get(deviceID string) (*Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.devices[deviceID]
	if !ok {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}
	cp := *d
	return &cp, nil
}

func (m *MemoryManager) UpdateLoginStatus(deviceID string, platform string, loggedIn bool, accountName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}

	for i, ls := range d.LoginStatuses {
		if ls.Platform == platform {
			d.LoginStatuses[i].LoggedIn = loggedIn
			d.LoginStatuses[i].LastCheck = time.Now()
			if accountName != "" {
				d.LoginStatuses[i].AccountName = accountName
			}
			if !loggedIn {
				d.HealthScore = max(0, d.HealthScore-20)
			}
			return nil
		}
	}
	// 新增平台登录态
	d.LoginStatuses = append(d.LoginStatuses, PlatformLoginStatus{
		Platform:  platform,
		LoggedIn:  loggedIn,
		LastCheck: time.Now(),
	})
	return nil
}

func (m *MemoryManager) Register(device Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.devices[device.ID]; ok {
		return fmt.Errorf("device %s already exists", device.ID)
	}
	device.Status = StatusIdle
	device.LastHeartbeat = time.Now()
	m.devices[device.ID] = &device
	return nil
}

func (m *MemoryManager) Stats() PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s := PoolStats{
		Total:      len(m.devices),
		ByPlatform: make(map[string]int),
	}

	totalHealth := 0
	for _, d := range m.devices {
		switch d.Status {
		case StatusIdle:
			s.Idle++
		case StatusBusy:
			s.Busy++
		case StatusOffline:
			s.Offline++
		case StatusMaintenance:
			s.Maintenance++
		case StatusBanned:
			s.Banned++
		}
		if d.AssignedPlatform != "" {
			s.ByPlatform[d.AssignedPlatform]++
		}
		totalHealth += d.HealthScore
	}
	if s.Total > 0 {
		s.AvgHealth = totalHealth / s.Total
	}
	return s
}

func (m *MemoryManager) Close() error { return nil }

func (m *MemoryManager) countByStatus(status string) int {
	count := 0
	for _, d := range m.devices {
		if d.Status == status {
			count++
		}
	}
	return count
}

// SimulateTask 模拟在设备上执行采集任务
func SimulateTask(deviceID string, platform string, duration time.Duration) error {
	time.Sleep(duration)
	// 模拟 10% 的失败率
	if rand.Float32() < 0.1 {
		return fmt.Errorf("task failed: device %s platform %s", deviceID, platform)
	}
	return nil
}
