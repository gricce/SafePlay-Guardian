BINARY := platform
PKG    := ./cmd/platform
DIST   := dist
ADDR   ?= 127.0.0.1:7878

.PHONY: build run test tidy fmt vet clean cross

build:
	go build -o $(DIST)/$(BINARY) $(PKG)

run:
	go run $(PKG) -addr $(ADDR)

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf $(DIST)

cross:
	GOOS=darwin  GOARCH=arm64 go build -o $(DIST)/darwin-arm64/$(BINARY)     $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o $(DIST)/darwin-amd64/$(BINARY)     $(PKG)
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/windows-amd64/$(BINARY).exe $(PKG)
