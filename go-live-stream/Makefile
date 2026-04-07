.PHONY: install dev build run clean

install:
	go mod tidy
	cd frontend && npm install

dev:
	@echo "Starting dev servers..."
	@trap 'kill 0' SIGINT; \
	  go run main.go & \
	  (cd frontend && npm run dev) & \
	  wait

build:
	cd frontend && npm run build
	go build -o bin/server main.go

run: build
	./bin/server

clean:
	rm -rf bin/ frontend/dist/
