.PHONY: build test run clean docker-up docker-up-mirror docker-down docker-logs test-publish test-e2e kafka-topics kafka-consume

WORKERS = dedup-worker device-worker rss-worker cleaner-worker rule-engine es-worker alert-worker misp-worker opencti-worker review-api

# Build all workers
build:
	@mkdir -p bin
	$(foreach w,$(WORKERS),go build -o bin/$(w) ./cmd/$(w);)
	@echo "All workers built in ./bin/"
	@ls -lh bin/

# Run tests
test:
	go test -v -race ./...

# Run a specific worker locally (requires Kafka on localhost:9092)
run-%:
	KAFKA_BROKERS=localhost:9092 RULES_PATH=./rules/rules.yaml go run ./cmd/$*

# Docker compose with optional mirror support
docker-up:
	REGISTRY=${REGISTRY:-docker.io} docker compose -f deploy/docker-compose.yml up -d

docker-up-mirror:
	REGISTRY=docker.1ms.run docker compose -f deploy/docker-compose.yml up -d

docker-down:
	cd deploy && docker compose down

docker-logs:
	cd deploy && docker compose logs -f

docker-logs-%:
	cd deploy && docker compose logs -f $*

# Send a test CVE message through the pipeline
test-publish:
	@echo '{"title":"CVE-2026-1234 0day detected on darkweb","content":"New critical vulnerability CVE-2026-1234 found on underground forum. C2 infrastructure at 45.33.32.156:443 and malicious domain evil-c2.com. MD5: a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6","source":"darkweb","url":"https://darkweb.example/post/123"}' | \
	docker exec -i $$(docker ps -q -f name=kafka) sh -c 'kafka-console-producer --bootstrap-server localhost:9092 --topic raw-data'
	@echo "Test message published to raw-data"

# Publish a duplicate to test dedup
test-publish-dup:
	@echo '{"title":"CVE-2026-1234 0day detected on darkweb","content":"New critical vulnerability CVE-2026-1234 found on underground forum. C2 infrastructure at 45.33.32.156:443 and malicious domain evil-c2.com. MD5: a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6","source":"darkweb","url":"https://darkweb.example/post/123"}' | \
	docker exec -i $$(docker ps -q -f name=kafka) sh -c 'kafka-console-producer --bootstrap-server localhost:9092 --topic raw-data'
	@echo "Duplicate test message published (should be dropped by dedup-worker)"

# End-to-end: publish test data and watch it flow
test-e2e:
	@echo "Publishing test CVE data..."
	@make test-publish 2>/dev/null
	@sleep 3
	@echo ""
	@echo "=== Watching pipeline (30s timeout) ==="
	@docker compose -f deploy/docker-compose.yml logs --tail=20 -f 2>/dev/null &
	@sleep 30; kill %1 2>/dev/null; echo "Done"

kafka-topics:
	cd deploy && docker compose exec kafka kafka-topics --bootstrap-server localhost:9092 --list

kafka-consume-%:
	cd deploy && docker compose exec kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic $* --from-beginning --max-messages 5

# Clean build artifacts
clean:
	rm -rf bin/
