FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /google-trends .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /google-trends /usr/bin/google-trends
ENTRYPOINT ["/usr/bin/google-trends"]
