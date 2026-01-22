build:
	GOOS=linux GOARCH=amd64 go build -o release/aiplan-$(VERSION)-amd64 -ldflags "-s -w -X main.version=$(VERSION)" -tags embedSPA aiplan.go/cmd/aiplan/main.go
	GOOS=linux GOARCH=arm64 go build -o release/aiplan-$(VERSION)-arm64 -ldflags "-s -w -X main.version=$(VERSION)" -tags embedSPA aiplan.go/cmd/aiplan/main.go
