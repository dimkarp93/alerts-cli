BINARY := alerts-cli
PKG := ./cmd/$(BINARY)
PREFIX ?= $(HOME)/.local

export GOWORK := off
export GOFLAGS := -mod=vendor

.PHONY: build fmt vet test check install clean vendor vendor-check bump-patch bump-minor bump-major _bump-commit

build:
	@v=$$(tr -d '[:space:]' < versions.txt); \
	u=$$(git remote get-url origin 2>/dev/null || true); \
	case "$$u" in \
		"")    o=local ;; \
		*://*) h=$${u#*://}; h=$${h#*@}; o="https://$${h%.git}" ;; \
		*:*)   h=$${u#*@};   o="https://$$(printf '%s' "$${h%.git}" | tr ':' '/')" ;; \
		*)     o=local ;; \
	esac; \
	if [ -f upstream.txt ]; then up=$$(tr -d '[:space:]' < upstream.txt); else up="$$o"; fi; \
	c=$$(git rev-parse --short HEAD 2>/dev/null || true); \
	CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X main.version=$$v -X main.origin=$$o -X main.upstream=$$up -X main.commit=$$c -X main.channel=local" \
		-o $(BINARY) $(PKG) && \
	echo "Built: ./$(BINARY) (v$$v)"

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

check:
	test -z "$$(gofmt -l cmd)" || { gofmt -l cmd; exit 1; }
	go vet ./...
	go test ./...

install: build
	install -D -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist

vendor:
	GOWORK=off go mod tidy
	GOWORK=off go mod vendor

vendor-check:
	GOWORK=off go mod vendor
	test -z "$$(git status --porcelain -- go.mod go.sum vendor/ | tee /dev/stderr)"

bump-patch:
	@v=$$(tr -d '[:space:]' < versions.txt); \
	MAJ=$${v%%.*}; rest=$${v#*.}; MIN=$${rest%%.*}; PAT=$${rest##*.}; \
	printf '%s.%s.%s\n' "$$MAJ" "$$MIN" "$$((PAT + 1))" > versions.txt; \
	cat versions.txt
	@$(MAKE) --no-print-directory _bump-commit LEVEL=patch

bump-minor:
	@v=$$(tr -d '[:space:]' < versions.txt); \
	MAJ=$${v%%.*}; rest=$${v#*.}; MIN=$${rest%%.*}; \
	printf '%s.%s.0\n' "$$MAJ" "$$((MIN + 1))" > versions.txt; \
	cat versions.txt
	@$(MAKE) --no-print-directory _bump-commit LEVEL=minor

bump-major:
	@v=$$(tr -d '[:space:]' < versions.txt); \
	MAJ=$${v%%.*}; \
	printf '%s.0.0\n' "$$((MAJ + 1))" > versions.txt; \
	cat versions.txt
	@$(MAKE) --no-print-directory _bump-commit LEVEL=major

_bump-commit:
	@v=$$(tr -d '[:space:]' < versions.txt); \
	if git rev-parse -q --verify "refs/tags/v$$v" >/dev/null; then \
		git checkout -- versions.txt; echo "tag v$$v already exists" >&2; exit 1; \
	fi; \
	git commit -q -m "bump $(LEVEL)" -- versions.txt && git tag "v$$v" || exit 1; \
	rc=0; for r in $$(git remote); do \
		git push -q "$$r" HEAD --tags || { echo "push to $$r failed" >&2; rc=1; }; \
	done; \
	echo "Tagged v$$v"; exit $$rc
