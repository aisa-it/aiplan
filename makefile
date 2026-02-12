build:
	GOOS=linux GOARCH=amd64 go build -o release/aiplan-$(VERSION)-amd64 -ldflags "-s -w -X main.version=$(VERSION)" -tags embedSPA aiplan.go/cmd/aiplan/main.go
	GOOS=linux GOARCH=arm64 go build -o release/aiplan-$(VERSION)-arm64 -ldflags "-s -w -X main.version=$(VERSION)" -tags embedSPA aiplan.go/cmd/aiplan/main.go

docsgen:
	cd aiplan.go; go generate ./...

fetch-front:
	cd aiplan-front; git pull; yarn && yarn build && cd .. && git add aiplan-front && git commit -m "front"
