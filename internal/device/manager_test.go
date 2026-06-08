package device

import (
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	// Acquire for douyin
	dev, err := mgr.Acquire(PlatformDouyin, "task-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if dev.Status != StatusBusy {
		t.Errorf("expected busy, got %s", dev.Status)
	}
	if dev.CurrentTask != "task-1" {
		t.Errorf("expected task-1, got %s", dev.CurrentTask)
	}

	// Acquire another
	dev2, err := mgr.Acquire(PlatformDouyin, "task-2")
	if err != nil {
		t.Fatalf("acquire second: %v", err)
	}
	if dev2.ID == dev.ID {
		t.Errorf("acquired same device twice")
	}

	// Release
	err = mgr.Release(dev.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Device should be idle again
	dev, _ = mgr.Get(dev.ID)
	if dev.Status != StatusIdle {
		t.Errorf("after release, expected idle, got %s", dev.Status)
	}
}

func TestAcquireNoDevice(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	// 把所有设备设为 offline
	devices, _ := mgr.List()
	for _, d := range devices {
		mgr.UpdateStatus(d.ID, StatusOffline)
	}

	_, err := mgr.Acquire(PlatformDouyin, "task-3")
	if err == nil {
		t.Error("expected error when no idle devices")
	}
}

func TestHeartbeatAndHealth(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	devices, _ := mgr.List()
	if len(devices) == 0 {
		t.Fatal("expected mock devices")
	}

	devID := devices[0].ID
	err := mgr.Heartbeat(devID, 50)
	if err != nil {
		t.Fatal(err)
	}

	dev, _ := mgr.Get(devID)
	if dev.HealthScore != 50 {
		t.Errorf("expected health 50, got %d", dev.HealthScore)
	}
	if dev.LastHeartbeat.IsZero() {
		t.Error("heartbeat time should be set")
	}
}

func TestLoginStatus(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	devices, _ := mgr.List()
	devID := devices[0].ID

	// Login expired
	err := mgr.UpdateLoginStatus(devID, PlatformDouyin, false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Should not be able to acquire for douyin (no login)
	_, err = mgr.Acquire(PlatformDouyin, "task-login")
	if err == nil {
		t.Error("expected acquire to fail when no valid login for platform")
	}

	// Re-login
	err = mgr.UpdateLoginStatus(devID, PlatformDouyin, true, "new_account")
	if err != nil {
		t.Fatal(err)
	}

	// Now should work
	_, err = mgr.Acquire(PlatformDouyin, "task-login-2")
	if err != nil {
		t.Errorf("expected acquire to work after re-login: %v", err)
	}
}

func TestPoolStats(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	stats := mgr.Stats()
	if stats.Total != 5 {
		t.Errorf("expected 5 devices, got %d", stats.Total)
	}
	if stats.Idle != 5 {
		t.Errorf("expected 5 idle, got %d", stats.Idle)
	}

	// Occupy one
	mgr.Acquire(PlatformDouyin, "task-stats")
	stats = mgr.Stats()
	if stats.Idle != 4 || stats.Busy != 1 {
		t.Errorf("expected 4 idle / 1 busy, got %d idle / %d busy", stats.Idle, stats.Busy)
	}
}

func TestRegisterNewDevice(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	newDev := Device{
		ID:        "device-new-1",
		Name:      "Xiaomi-Redmi-01",
		ADBSerial: "adb-xxxx",
		Tags:      []string{"production", "douyin"},
	}
	err := mgr.Register(newDev)
	if err != nil {
		t.Fatal(err)
	}

	dev, _ := mgr.Get("device-new-1")
	if dev.Status != StatusIdle {
		t.Errorf("new device should be idle, got %s", dev.Status)
	}

	// Duplicate
	err = mgr.Register(newDev)
	if err == nil {
		t.Error("expected error for duplicate device")
	}
}

func TestReleaseRestoresHealth(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	devices, _ := mgr.List()
	dev := &devices[0]
	origHealth := dev.HealthScore

	// Acquire and release should slightly restore health
	mgr.Acquire(PlatformDouyin, "test")
	mgr.Release(dev.ID)

	devAfter, _ := mgr.Get(dev.ID)
	if devAfter.HealthScore < origHealth {
		t.Errorf("health should not decrease after release (was %d, got %d)", origHealth, devAfter.HealthScore)
	}
}

func TestConcurrentAcquire(t *testing.T) {
	mgr := NewMemoryManager()
	defer mgr.Close()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			dev, err := mgr.Acquire(PlatformDouyin, fmt.Sprintf("concurrent-%d", id))
			if err == nil {
				time.Sleep(10 * time.Millisecond)
				mgr.Release(dev.ID)
			}
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := mgr.Stats()
	if stats.Idle != 5 {
		t.Errorf("after concurrent test, expected all 5 idle, got %d", stats.Idle)
	}
}
