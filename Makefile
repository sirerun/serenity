# GOWORK=off: serenity is a standalone module; keep any parent go.work
# (e.g. the sirerun multi-repo workspace) from leaking into builds.
GO := GOWORK=off go

.PHONY: build test vet fmt

build:
	CGO_ENABLED=0 $(GO) build ./cmd/serenity

test:
	$(GO) test -race -timeout 600s ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .
