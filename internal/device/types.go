package device

import "time"

// 设备状态
const (
	StatusIdle       = "idle"
	StatusBusy       = "busy"
	StatusMaintenance = "maintenance"
	StatusOffline    = "offline"
	StatusBanned     = "banned"
)

// 平台
const (
	PlatformDouyin     = "douyin"
	PlatformKuaishou   = "kuaishou"
	PlatformXiaohongshu = "xiaohongshu"
	PlatformXianyu     = "xianyu"
)

type PlatformLoginStatus struct {
	Platform    string    `json:"platform"`
	LoggedIn    bool      `json:"logged_in"`
	AccountName string    `json:"account_name,omitempty"`
	ValidUntil  time.Time `json:"valid_until,omitempty"` // 登录有效期
	LastCheck   time.Time `json:"last_check"`
	Note        string    `json:"note,omitempty"` // 封禁原因等
}

type Device struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	ADBSerial      string                  `json:"adb_serial"`
	Status         string                  `json:"status"`
	CurrentTask    string                  `json:"current_task,omitempty"`
	AssignedPlatform string                `json:"assigned_platform,omitempty"`
	LoginStatuses  []PlatformLoginStatus   `json:"login_statuses"`
	LastHeartbeat  time.Time               `json:"last_heartbeat"`
	HealthScore    int                     `json:"health_score"` // 0-100
	IP             string                  `json:"ip,omitempty"`
	SIMCard        string                  `json:"sim_card,omitempty"`
	Tags           []string                `json:"tags,omitempty"`
}

// ── 任务 ──

type TaskType string

const (
	TaskCollect    TaskType = "collect"     // 采集
	TaskCheckLogin TaskType = "check_login" // 检查登录态
	TaskMaintain   TaskType = "maintain"    // 维护
)

type Task struct {
	ID          string    `json:"id"`
	Type        TaskType  `json:"type"`
	Platform    string    `json:"platform"`
	DeviceID    string    `json:"device_id,omitempty"` // 空表示未分配
	Status      string    `json:"status"`              // pending, running, done, failed
	Priority    int       `json:"priority"`            // 0-100, 越高越优先
	Params      map[string]string `json:"params,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	AssignedAt  time.Time `json:"assigned_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}
