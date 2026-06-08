package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/argus-platform/argus/internal/broker"
	"github.com/argus-platform/argus/internal/device"
	"github.com/argus-platform/argus/internal/types"
	"github.com/google/uuid"
)

func main() {
	brokers := []string{getEnv("KAFKA_BROKERS", "localhost:9092")}
	groupID := "device-worker"

	bus, err := broker.New(brokers, groupID)
	if err != nil {
		log.Fatalf("connect kafka: %v", err)
	}
	defer bus.Close()

	// 设备状态管理器
	mgr := device.NewMemoryManager()
	defer mgr.Close()

	log.Println("[device-worker] started, managing device pool...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		cancel()
	}()

	// 定时上报设备池统计
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stats := mgr.Stats()
				data, _ := json.Marshal(stats)
				log.Printf("[device-worker] pool stats: %s", string(data))
				bus.Publish("telemetry", "device-stats", stats)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 模拟设备心跳
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				devices, _ := mgr.List()
				for _, d := range devices {
					mgr.Heartbeat(d.ID, d.HealthScore)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 从 task-queue topic 接收采集任务
	err = bus.Consume(ctx, []string{"task-queue"}, func(msg []byte) error {
		var task device.Task
		if err := json.Unmarshal(msg, &task); err != nil {
			log.Printf("unmarshal task error: %v", err)
			return nil
		}

		log.Printf("[device-worker] received task: %s (platform=%s, type=%s)",
			task.ID, task.Platform, task.Type)

		// 分配设备
		dev, err := mgr.Acquire(task.Platform, task.ID)
		if err != nil {
			log.Printf("[device-worker] no device for task %s: %v", task.ID, err)
			// 延迟重试: 把任务放回队列
			time.Sleep(5 * time.Second)
			return bus.Publish("task-queue", task.ID, task)
		}

		log.Printf("[device-worker] device %s assigned to task %s", dev.Name, task.ID)

		// 执行采集
		var collectErr error
		switch task.Platform {
		case device.PlatformDouyin:
			collectErr = collectPlatform(dev, "抖音", task.Params)
		case device.PlatformKuaishou:
			collectErr = collectPlatform(dev, "快手", task.Params)
		case device.PlatformXiaohongshu:
			collectErr = collectPlatform(dev, "小红书", task.Params)
		case device.PlatformXianyu:
			collectErr = collectPlatform(dev, "闲鱼", task.Params)
		default:
			collectErr = collectPlatform(dev, task.Platform, task.Params)
		}

		// 释放设备
		mgr.Release(dev.ID)

		if collectErr != nil {
			log.Printf("[device-worker] task %s failed on %s: %v", task.ID, dev.Name, collectErr)
			// 标记设备健康下降
			mgr.Heartbeat(dev.ID, dev.HealthScore-10)
			return nil
		}

		log.Printf("[device-worker] task %s completed on %s", task.ID, dev.Name)

		// 发布采集结果到 raw-data
		raw := types.RawData{
			ID:        uuid.New().String(),
			Source:    "device-farm",
			Platform:  task.Platform,
			Collected: time.Now(),
			Metadata: map[string]string{
				"device_id":   dev.ID,
				"device_name": dev.Name,
				"task_id":     task.ID,
			},
		}
		return bus.Publish("raw-data", raw.ID, raw)
	})

	log.Printf("[device-worker] stopped: %v", err)
}

func collectPlatform(dev *device.Device, platformName string, params map[string]string) error {
	// 在实际环境中，这里会通过 ADB + uiautomator2 控制真机
	// 当前为模拟实现
	keyword := params["keyword"]
	if keyword == "" {
		keyword = "default"
	}
	log.Printf("[device-worker] collecting %s on device %s (keyword=%s)...", platformName, dev.Name, keyword)
	time.Sleep(2 * time.Second) // 模拟采集耗时

	// 模拟 5% 故障率
	// if rand.Float32() < 0.05 {
	//     return fmt.Errorf("device error: lost adb connection")
	// }

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
