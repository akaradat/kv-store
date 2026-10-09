STATICCHECK ?= go run honnef.co/go/tools/cmd/staticcheck@v0.8.1
DLV ?= go run github.com/go-delve/delve/cmd/dlv@v1.27.1
DLV_FLAGS = --headless --listen=127.0.0.1:2345 --api-version=2 --accept-multiclient --continue
URL ?= http://localhost:7000

.PHONY: check test e2e run-node counter debug-node debug-router

check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet -tags e2e ./...
	$(STATICCHECK) -tags e2e ./...
	@scripts/check-arch.sh
	go test -race ./...

test:
	go test -race ./...

# Builds the real binaries and runs them as separate processes; not part of check.
e2e:
	go test -race -tags e2e ./test/e2e

run-node:
	go run ./cmd/kvnode -addr :7000 -node-id node-1

counter:
	go run ./test/e2e/kvcounter -url $(URL)

# The debug targets start the binary under Delve on 127.0.0.1:2345, already running; attach an editor to set
# breakpoints (VS Code: "Attach to make debug-*").
debug-node:
	$(DLV) debug ./cmd/kvnode $(DLV_FLAGS) -- -addr :7000 -node-id node-1

# Needs the nodes on localhost:7001–7003 first: docker compose up -d node-1 node-2 node-3
debug-router:
	$(DLV) debug ./cmd/kvrouter $(DLV_FLAGS) -- -addr :7000 -cluster cluster.json
