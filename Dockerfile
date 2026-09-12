# Stage 1: build
FROM golang:1.26 AS builder

WORKDIR /src

#Copy dependency files
COPY go.mod go.sum ./

RUN go mod download

#Copy the source
COPY . .

# Build a static Linux binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /sentinel-agent ./agent

# Stage 2: the final tiny image
FROM gcr.io/distroless/static-debian12

COPY --from=builder /sentinel-agent /sentinel-agent

USER nonroot:nonroot

ENTRYPOINT ["/sentinel-agent"]

# Build a static Linux binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /sentinel-agent ./agent

# Stage 2: the final tiny image
FROM gcr.io/distroless/static-debian12

COPY --from=builder /sentinel-agent /sentinel-agent

USER nonroot:nonroot

ENTRYPOINT ["/sentinel-agent"]