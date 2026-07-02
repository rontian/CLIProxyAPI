FROM golang:1.26-bookworm AS builder

WORKDIR /app

ARG DEBIAN_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=off
ARG GITHUB_PROXY_PREFIX=https://ghfast.top/https://github.com

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN if [ -n "$DEBIAN_MIRROR" ]; then sed -i "s#deb.debian.org#${DEBIAN_MIRROR}#g" /etc/apt/sources.list.d/debian.sources; fi && \
    apt-get update && \
    apt-get install -y --no-install-recommends build-essential git && \
    rm -rf /var/lib/apt/lists/*

RUN if [ -n "$GITHUB_PROXY_PREFIX" ]; then git config --global url."${GITHUB_PROXY_PREFIX}".insteadOf "https://github.com"; fi

COPY go.mod go.sum ./

RUN go mod download -x

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN if [ "$BUILD_DATE" = "unknown" ] || [ -z "$BUILD_DATE" ]; then BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ'); fi && \
    CGO_ENABLED=1 GOOS=linux go build -buildvcs=false -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./CLIProxyAPI ./cmd/server/

RUN mkdir -p ./plugins/linux/$(go env GOARCH) && \
    CGO_ENABLED=1 GOOS=linux go build -buildmode=c-shared -o ./plugins/linux/$(go env GOARCH)/github-copilot.so ./plugins-src/github-copilot/go


FROM debian:bookworm

ARG DEBIAN_MIRROR=mirrors.aliyun.com

RUN if [ -n "$DEBIAN_MIRROR" ]; then sed -i "s#deb.debian.org#${DEBIAN_MIRROR}#g" /etc/apt/sources.list.d/debian.sources; fi && \
    apt-get update && \
    apt-get install -y --no-install-recommends tzdata ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir /CLIProxyAPI

COPY --from=builder ./app/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI
COPY --from=builder ./app/plugins /CLIProxyAPI/plugins

COPY config.example.yaml /CLIProxyAPI/config.example.yaml

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./CLIProxyAPI"]
