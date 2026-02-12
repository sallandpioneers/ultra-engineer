FROM golang:1.23-bookworm AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o ultra-engineer ./cmd/ultra-engineer

FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        git \
        ca-certificates \
        curl \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends gh \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code

COPY --from=builder /build/ultra-engineer /usr/local/bin/ultra-engineer

RUN usermod -l engineer -d /home/engineer -m node \
    && groupmod -n engineer node
USER engineer
RUN git config --global user.name "Ultra Engineer" \
    && git config --global user.email "ultra-engineer@noreply"
WORKDIR /home/engineer

ENTRYPOINT ["ultra-engineer"]
CMD ["daemon"]
