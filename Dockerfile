FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/hostwatch-server ./cmd/hostwatch-server && \
    CGO_ENABLED=0 go build -o /out/hostwatch-agent ./cmd/hostwatch-agent

FROM alpine:3.21
RUN adduser -D -H appuser
COPY --from=builder /out/hostwatch-server /usr/local/bin/hostwatch-server
COPY --from=builder /out/hostwatch-agent /usr/local/bin/hostwatch-agent
USER appuser
EXPOSE 8096
CMD ["hostwatch-server", "-addr", "0.0.0.0:8096"]
