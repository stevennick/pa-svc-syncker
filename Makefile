VERSION_MAJOR ?= 0
VERSION_MINOR ?= 6
VERSION_BUILD ?= 5
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)

GOOS ?= $(shell go env GOOS)

ORG := github.com
OWNER := inwinstack
REPOPATH ?= $(ORG)/$(OWNER)/pa-svc-syncker

$(shell mkdir -p ./out)

.PHONY: build
build: out/controller

.PHONY: out/controller
out/controller:
	GOOS=$(GOOS) go build \
	  -ldflags="-s -w -X $(REPOPATH)/pkg/version.version=$(VERSION)" \
	  -a -o $@ cmd/main.go

.PHONY: dep 
dep:
	dep ensure

.PHONY: test
test:
	./hack/test-go.sh

.PHONY: build_image
build_image:
	docker build -t $(OWNER)/pa-svc-syncker:$(VERSION) .

.PHONY: push_image
push_image:
	docker push $(OWNER)/pa-svc-syncker:$(VERSION)

.PHONY: clean
clean:
	rm -rf out/

