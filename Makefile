.PHONY: test build schemas web
test:
	go test ./...
schemas:
	go run ./scripts/genschema
web:
	cd web && pnpm install --frozen-lockfile && pnpm build
build: web
	go build -trimpath -o bin/malta ./cmd/malta
