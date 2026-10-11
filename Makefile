IMAGE ?= ghcr.io/nobloat/poll
TAG ?= $(or $(shell git describe --tags --exact-match 2>/dev/null),main)

.PHONY: dev mailpit test build build-upx package push screenshots clean
dev: mailpit
	test -f .env || cp .env.sample .env
	test -d data/polls || { mkdir -p data/polls && cp seed/*.json data/polls/; }
	set -a && . ./.env && set +a && go run .

mailpit:
	docker start nobloat-poll-mailpit 2>/dev/null || docker run -d --rm --name nobloat-poll-mailpit -p 1025:1025 -p 8025:8025 axllent/mailpit:v1.27
	until curl -sf localhost:8025/readyz >/dev/null; do sleep .2; done

test:
	go vet ./... && go test -race ./...

build:
	CGO_ENABLED=0 go build -tags timetzdata,nethttpomithttp2 -trimpath -ldflags="-s -w -X main.release=$(TAG)" -o poll .
	@du -h poll | cut -f1 | xargs echo "binary size:"

build-upx: build
	upx -q --best poll
	@du -h poll | cut -f1 | xargs echo "compressed size:"

package:
	docker build -t $(IMAGE):$(TAG) .
	@docker image ls $(IMAGE):$(TAG) --format "image size: {{.Size}}"

push: package
	docker push $(IMAGE):$(TAG)

screenshots:
	./screenshots.sh

clean:
	rm -rf data/* screenshots/*/
