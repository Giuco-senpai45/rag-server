FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o ./rag-server main.go

FROM alpine

COPY --from=builder /app/rag-server ./rag-server

EXPOSE 8080

CMD ["./rag-server"]