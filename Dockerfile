FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/gobl-dev .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /usr/local/bin/gobl-dev /usr/local/bin/gobl-dev
ENTRYPOINT ["gobl-dev"]
