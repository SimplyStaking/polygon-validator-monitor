MAIN_PACKAGE_PATH := ./cmd/monitor
BINARY_NAME := polygon_monitor

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## build: build the application
.PHONY: build
build:
	go build -o=./build/bin/${BINARY_NAME} ${MAIN_PACKAGE_PATH}

## run: run the  application
.PHONY: run
run: build
	./build/bin/${BINARY_NAME}