.PHONY: web-dev mobile-dev web mobile build-frontend build-pc build-mobile kill-port run-backend run run-prod stop install init

kill-port:
	@fuser -k 5556/tcp 2>/dev/null || true
	@fuser -k 5557/tcp 2>/dev/null || true

build-frontend: build-pc build-mobile

build-pc:
	@echo "Building PC frontend..."
	cd frontend-pc && npm install && npm run build

build-mobile:
	@echo "Building Mobile frontend..."
	cd frontend-mobile && npm install && npm run build

web: build-pc
	@echo "PC frontend build completed."

mobile: build-mobile
	@echo "Mobile frontend build completed."

run-backend: kill-port
	@echo "=========================================="
	@echo "Starting backend services..."
	@echo "Backend API (Mobile): http://localhost:5556"
	@echo "Backend API (PC):     http://localhost:5557"
	@echo "=========================================="
	cd backend && go run main.go &

web-dev:
	@echo "Starting PC frontend development server..."
	@echo "PC Frontend: http://localhost:5554 (with source map)"
	@cd frontend-pc && npm run dev

mobile-dev:
	@echo "Starting Mobile frontend development server..."
	@echo "Mobile Frontend: http://localhost:5553"
	@cd frontend-mobile && npm run dev

run: run-backend

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

# Prerelease targets
.PHONY: prerelease clean-prerelease prebuild-pc prebuild-mobile prebuild-backend

clean-prerelease:
	@echo "Cleaning prerelease directories..."
clean-prerelease:
	@echo "Cleaning prerelease directories..."
	rm -rf prerelease/www prerelease/mobile prerelease/service

prebuild-pc: clean-prerelease
	@echo "Building PC frontend for prerelease..."
	@mkdir -p prerelease/www
	cd frontend-pc && VITE_API_BASE_URL=/api VITE_BEACONIAM_EXTERNAL_URL=https://iam.cadenzayueqi.com VITE_IAM_PC_CLIENT_ID=tuneloop_web VITE_IAM_PC_REDIRECT_URI=https://web.cadenzayueqi.com/callback npm run build
	@cp -r frontend-pc/dist/* prerelease/www/

prebuild-mobile:
	@echo "Building Mobile frontend for prerelease..."
	@mkdir -p prerelease/mobile
	cd frontend-mobile && npm run build -- --mode prerelease
	@cp -r frontend-mobile/dist/* prerelease/mobile/

prebuild-backend:
	@echo "Building backend for prerelease..."
	@mkdir -p prerelease/service
	cd backend && go build -o ../prerelease/service/tuneloop .

prerelease: clean-prerelease prebuild-backend prebuild-pc prebuild-mobile
	@echo "=========================================="
	@echo "预发布构建完成"
	@echo "PC:   web.cadenzayueqi.com"
	@echo "WX:   wx.cadenzayueqi.com"
	@echo "=========================================="
