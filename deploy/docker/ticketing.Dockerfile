# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build
WORKDIR /src
# Copy truoc phan manifest de tan dung cache layer: doi code khong lam tai lai module.
COPY go.work ./
COPY libs/go/go.mod libs/go/
COPY services/ticketing/go.mod services/ticketing/
COPY libs ./libs
COPY services/ticketing ./services/ticketing
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/ticketing && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/server"]
