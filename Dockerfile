# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

RUN mkdir -p /out/uploads

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
# Owned by the non-root user; a volume mounted here inherits this on first use.
COPY --from=build --chown=nonroot:nonroot /out/uploads /data/uploads
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
