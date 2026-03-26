.PHONY: web-dev build-frontend build-pc build-mobile run-backend run run-frontend run-prod stop install init

web-dev: build-frontend run-backend

build-frontend: build-pc build-mobile

build-pc:
	@echo "Building PC frontend..."
	cd frontend-pc && npm install && npm run build

build-mobile:
	@echo "Building Mobile frontend..."
	cd frontend-mobile && npm install && npm run build

run-backend:
	@echo "=========================================="
	@echo "Starting backend services..."
	@echo "Backend API:    http://localhost:5556 (PC Backend)"
	@echo "PC Frontend:    http://localhost:5554 (with source map)"
	@echo "Mobile Frontend: http://localhost:5553"
	@echo "=========================================="
	cd backend && go run main.go &

run-frontend:
	@echo "Starting PC frontend development server..."
	@echo "PC Frontend: http://localhost:5554 (with source map)"
	@cd frontend-pc && VITE_DEV_PORT=5554 npm run dev

run: run-backend
	@echo "=========================================="
	@echo "Starting PC frontend development server..."
	@echo "PC Frontend: http://localhost:5554 (with source map)"
	@echo "=========================================="
	cd frontend-pc && VITE_DEV_PORT=5554 npm run dev &

run-prod: build-frontend run-backend

stop:
	@echo "Stopping all services..."
	@pkill -f "go run main.go" || true
	@pkill -f "npm run dev" || true

install:
	@echo "Installing backend dependencies..."
	cd backend && go mod tidy
	@echo "Installing PC frontend dependencies..."
	cd frontend-pc && npm install
	@echo "Installing mobile frontend dependencies..."
	cd frontend-mobile && npm install

init: install
	@echo "Running database migrations..."
	cd backend && go run cmd/migrate/main.go