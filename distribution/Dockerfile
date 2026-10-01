FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=0.0.0-dev
RUN GOWORK=off CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go -C tui build -trimpath -ldflags="-s -w -X github.com/underpass-ai/AXLR/buildinfo.Version=$VERSION" -o /out/axlr-serve ./cmd/axlr-serve

FROM --platform=$BUILDPLATFORM python:3.13-slim AS engines
WORKDIR /src
COPY distribution/engines.lock.json distribution/fetch_engines.py ./distribution/
ARG TARGETARCH
RUN python distribution/fetch_engines.py --lock distribution/engines.lock.json --arch "$TARGETARCH" --out /out

FROM gcr.io/distroless/cc-debian13:nonroot
COPY --from=build /out/axlr-serve /usr/local/bin/axlr-serve
COPY --from=engines /out/kmp-mcp /usr/local/bin/kmp-mcp
COPY --from=engines /out/made-mcp /usr/local/bin/made-mcp
COPY LICENSE NOTICE /licenses/axlr/
COPY distribution/third_party /licenses/third_party
COPY distribution/engine_licenses /licenses/engines
USER 65532:65532
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/axlr-serve"]
CMD ["--config=/etc/axlr/service.json"]
