FROM --platform=linux/amd64 golang:1.24 AS builder

# install C toolchain & sqlite headers (Debian)
RUN apt-get update && apt-get install -y \
    build-essential \
    libsqlite3-dev \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# 1) copy module files & download deps
COPY go.mod go.sum ./
RUN go mod download

# 2) copy the rest of the sources
COPY . .

# 3) ensure output dir exists and build
RUN mkdir -p /out \
 && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/devstreamlinebot main.go

FROM scratch
COPY --from=builder /out/devstreamlinebot /usr/local/bin/devstreamlinebot
ENTRYPOINT ["/usr/local/bin/devstreamlinebot"]