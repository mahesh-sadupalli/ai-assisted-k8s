# One multi-stage Dockerfile for every Go binary in the repo:
#   docker build --build-arg CMD=control-plane -t ghcr.io/mahesh-sadupalli/control-plane:dev .
#   docker build --build-arg CMD=services/hello -t ghcr.io/mahesh-sadupalli/hello:dev .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
