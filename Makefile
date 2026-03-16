.PHONY: web-dev build-frontend build-pc build-mobile run-backend

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
	@echo "PC Frontend:    http://localhost:5554"
	@echo "Mobile Frontend: http://localhost:5553"
	@echo "=========================================="
	cd backend && go run main.go