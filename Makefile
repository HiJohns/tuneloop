.PHONY: web-dev build-frontend run-backend

web-dev: build-frontend run-backend

build-frontend:
	@echo "Building frontend..."
	cd frontend-mobile && npm install && npm run build

run-backend:
	@echo "Starting backend server on port 5554..."
	cd backend && go run main.go
