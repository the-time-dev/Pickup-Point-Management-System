FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o avito_intr ./cmd/main.go

FROM debian:bookworm-slim

ENV PORT=8080

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

RUN useradd -m appuser
USER appuser

COPY --from=builder /app/avito_intr /avito_intr

EXPOSE 8080

ENTRYPOINT ["/avito_intr"]
