# syntax=docker/dockerfile:1
# Dockerfile chung cho cac service Go "don binary": gateway, identity, event, payment, notification.
#   docker build -f deploy/docker/go-service.Dockerfile --build-arg SERVICE=gateway .
# ticketing va waitingroom co Dockerfile rieng vi moi cai dong goi them binary worker/controller.
FROM golang:1.26-alpine AS build
ARG SERVICE
WORKDIR /src
# Workspace Go doc go.mod cua MOI module trong go.work, nen phai copy du. Danh sach nay phai khop
# `go.work`. go.sum dung glob (`go.sum*`) vi mot so module (notification, antibot) chua co go.sum.
COPY go.work go.work.sum ./
COPY services/gateway/go.mod services/gateway/go.sum* services/gateway/
COPY services/identity/go.mod services/identity/go.sum* services/identity/
COPY services/event/go.mod services/event/go.sum* services/event/
COPY services/waitingroom/go.mod services/waitingroom/go.sum* services/waitingroom/
COPY services/ticketing/go.mod services/ticketing/go.sum* services/ticketing/
COPY services/payment/go.mod services/payment/go.sum* services/payment/
COPY services/antibot/go.mod services/antibot/go.sum* services/antibot/
COPY services/notification/go.mod services/notification/go.sum* services/notification/
COPY libs ./libs
COPY services/${SERVICE} ./services/${SERVICE}
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/${SERVICE} && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
