PWD := $(shell pwd)

prepare:
	@docker build -t buildertools/drone-rancher:build-tooling -f tooling.df .

update-deps:
	@docker run --rm -v $(PWD):/go/src/github.com/buildertools/drone-rancher -w /go/src/github.com/buildertools/drone-rancher buildertools/drone-rancher:build-tooling trash -u
update-vendor:
	@docker run --rm -v $(PWD):/go/src/github.com/buildertools/drone-rancher -w /go/src/github.com/buildertools/drone-rancher buildertools/drone-rancher:build-tooling trash

test:
	@docker run --rm \
	  -v $(PWD):/go/src/github.com/buildertools/drone-rancher \
	  -v $(PWD)/bin:/go/bin \
	  -v $(PWD)/pkg:/go/pkg \
	  -v $(PWD)/reports:/go/reports \
	  -w /go/src/github.com/buildertools/drone-rancher \
	  golang:1.7 \
	  go test -cover ./...
	  
build:
	@docker run --rm \
	  -v $(PWD):/go/src/github.com/buildertools/drone-rancher \
	  -v $(PWD)/bin:/go/bin \
	  -v $(PWD)/pkg:/go/pkg \
	  -w /go/src/github.com/buildertools/drone-rancher \
	  -e GOOS=darwin \
	  -e GOARCH=amd64 \
	  -e CGO_ENABLED=0 \
	  golang:1.7 \
	  go build -o bin/drone-rancher-darwin64
	 @docker run --rm \
	  -v $(PWD):/go/src/github.com/buildertools/drone-rancher \
	  -v $(PWD)/bin:/go/bin \
	  -v $(PWD)/pkg:/go/pkg \
	  -w /go/src/github.com/buildertools/drone-rancher \
	  -e GOOS=linux \
	  -e GOARCH=amd64 \
	  -e CGO_ENABLED=0 \
	  golang:1.7 \
	  go build -o bin/drone-rancher-linux64
	  
