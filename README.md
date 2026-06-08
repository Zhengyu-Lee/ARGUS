# ARGUS — Advanced Reconnaissance & Global Unified Surveillance

百眼巨人全景威胁情报与舆情融合平台

## 架构

```
采集层 ──→ Kafka ──→ 清洗 Worker ──→ Kafka ──→ 规则引擎 ──┬─→ ES (全部数据)
(RSS/群控)                                                    ├─→ MISP (置信度>80)
                                                             ├─→ OpenCTI (置信度>80)
                                                             └─→ 审核队列 (60-80分)
```

## 快速启动

### 前置条件
- Go 1.22+
- Docker + Docker Compose (可选，用于 Kafka)

### 1. 启动 Kafka (Docker)

```bash
# 国内用户加镜像前缀
REGISTRY=docker.1ms.run docker compose -f deploy/docker-compose.yml up -d kafka kafka-init redpanda-console
```

### 2. 运行 Worker (本地开发)

```bash
# 终端1: RSS Worker (模拟数据入口)
go run ./cmd/rss-worker

# 终端2: 清洗 Worker
go run ./cmd/cleaner-worker

# 终端3: 规则引擎
RULES_PATH=./rules/rules.yaml go run ./cmd/rule-engine
```

### 3. 发送测试数据

```bash
# 通过 Kafka 发一条测试消息
echo '{"title":"CVE-2026-1234","content":"New 0day CVE-2026-1234 on darkweb, C2 at 45.33.32.156:443","source":"darkweb"}' | \
docker exec -i argus-kafka-1 sh -c 'kafka-console-producer.sh --bootstrap-server localhost:9092 --topic raw-data'
```

### 4. 运行测试

```bash
go test -v -race ./...
```

## 项目结构

```
argus/
├── cmd/                    # Worker 入口
│   ├── rss-worker/        # RSS 采集 Worker
│   ├── cleaner-worker/    # 数据清洗 + IoC 提取
│   ├── rule-engine/       # 规则决策引擎
│   ├── misp-worker/       # MISP 推送 Worker
│   └── opencti-worker/    # OpenCTI 推送 Worker
├── internal/              # 内部包
│   ├── types/             # 数据模型
│   ├── broker/            # Kafka 消息总线
│   ├── rules/             # 规则引擎
│   └── extractor/         # IoC 提取器
├── rules/
│   └── rules.yaml         # 规则配置文件
├── deploy/
│   ├── docker-compose.yml
│   └── Dockerfile.worker
├── Makefile               # 快捷命令
└── README.md
```

## 数据流

```
raw-data ──→ rss-worker ──→ enriched ──→ cleaner-worker ──→ ioc-candidate
                                                                │
                                                          rule-engine
                                                         ┌────┴────┐
                                                         │         │
                                                  置信度>80     60-80
                                                         │         │
                                                  ioc-confirmed  human-review
                                                  ┌───┴───┐         │
                                                  │       │    Workflow Engine
                                           misp-worker  opencti-worker
                                               MISP       OpenCTI
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka 地址 |
| `RULES_PATH` | `/etc/argus/rules.yaml` | 规则文件路径 |
| `MISP_URL` | `http://localhost:8080` | MISP 地址 |
| `MISP_KEY` | - | MISP API Key |
| `OPENCTI_URL` | `http://localhost:4000` | OpenCTI 地址 |
| `OPENCTI_TOKEN` | - | OpenCTI API Token |
| `REGISTRY` | `docker.io` | Docker 镜像仓库 (国内用 `docker.1ms.run`) |
