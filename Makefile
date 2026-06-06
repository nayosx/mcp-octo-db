APP_NAME := octo-db
DIST_DIR := dist

.PHONY: build build-all test vet clean checksums

build:
	mkdir -p $(DIST_DIR)
	go build -o $(DIST_DIR)/$(APP_NAME) .

build-all:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe .

test:
	go test ./...

vet:
	go vet ./...

checksums:
	cd $(DIST_DIR) && sha256sum * > checksums.txt

clean:
	rm -rf $(DIST_DIR)
