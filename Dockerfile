FROM golang:1.27 AS build
WORKDIR /src
COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends upx-ucl && rm -rf /var/lib/apt/lists/*
RUN go vet ./... && go test ./... \
 && make build-upx && mv poll /poll \
 && mkdir -p /data

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /poll /poll
COPY --from=build --chown=65532:65532 --chmod=700 /data /data
WORKDIR /
VOLUME /data
EXPOSE 8737
USER 65532:65532
ENTRYPOINT ["/poll"]
