FROM golang:1.27 AS build
WORKDIR /src
COPY . .

RUN go vet ./... && go test ./... \
 && CGO_ENABLED=0 go build -tags timetzdata,nethttpomithttp2 -trimpath -ldflags="-s -w" -o /poll . \
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
