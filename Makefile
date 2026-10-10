IMAGE ?= ghcr.io/nobloat/poll
TAG ?= latest

.PHONY: dev mailpit test build package push screenshots clean
# local dev: Mailpit in Docker (UI :8025), the app via go run with .env (created from .env.sample);
# an empty data dir gets the example polls from seed/ (rm -rf data resets them)
dev: mailpit
	test -f .env || cp .env.sample .env
	test -d data/polls || { mkdir -p data/polls && cp seed/*.json data/polls/; }
	set -a && . ./.env && set +a && go run .

# catches all mail: SMTP on :1025, web UI on http://localhost:8025 (stop: docker stop nobloat-poll-mailpit)
mailpit:
	docker start nobloat-poll-mailpit 2>/dev/null || docker run -d --rm --name nobloat-poll-mailpit -p 1025:1025 -p 8025:8025 axllent/mailpit:v1.27
	until curl -sf localhost:8025/readyz >/dev/null; do sleep .2; done

test:
	go vet ./... && go test -race ./...

# static, stripped binary with the time zone database embedded
build:
	CGO_ENABLED=0 go build -tags timetzdata,nethttpomithttp2 -trimpath -ldflags="-s -w" -o poll .
	@du -h poll | cut -f1 | xargs echo "binary size:"

# Docker image (FROM scratch; the build stage runs the tests)
package:
	docker build -t $(IMAGE):$(TAG) .
	@docker image ls $(IMAGE):$(TAG) --format "image size: {{.Size}}"

# push to the GitHub container registry (once: docker login ghcr.io)
push: package
	docker push $(IMAGE):$(TAG)

# every page × theme × desktop/phone × light/dark from the seed data → screenshots/
screenshots:
	./screenshots.sh

# build output only; never touches ./data
clean:
	rm -rf poll screenshots/*/
