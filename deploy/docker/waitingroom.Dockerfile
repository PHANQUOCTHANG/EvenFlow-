# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
# `go.work` khai `use` CA 9 module. O che do workspace Go phai doc duoc go.mod cua TAT CA
# module do, nen ban cu -- chi copy libs/go va waitingroom -- fail ngay o lenh dau voi
# `go: cannot load module ../gateway listed in go.work file` (tai hien tai cho tu ban
# export sach). Image nay CHUA TUNG build duoc: Trivy matrix waitingroom do tren moi push
# len main, va nhieu kha nang day cung la ly do E2E nightly do o `make up` (chua tai hien
# rieng).
#
# Chi copy go.mod cua cac module KHONG dung, khong copy ma nguon cua chung: copy ca
# `services/` se keo file chua track cua moi service (`.env`, `*.pem`) vao build stage, va
# sua bat ky service nao cung lam mat cache cua image nay. Danh sach phai khop `go.work`:
# them module vao go.work thi them mot dong o day.
#
# Phai copy ca go.sum cua module con lai: workspace gop go.sum cua moi module, thieu thi
# `go build` bao `missing go.sum entry for go.mod file` du khong dung dependency do.
COPY go.work go.work.sum ./
COPY services/gateway/go.mod services/gateway/
COPY services/identity/go.mod services/identity/
COPY services/event/go.mod services/event/
COPY services/ticketing/go.mod services/ticketing/go.sum services/ticketing/
COPY services/payment/go.mod services/payment/
COPY services/antibot/go.mod services/antibot/
COPY services/notification/go.mod services/notification/
COPY libs ./libs
COPY services/waitingroom ./services/waitingroom
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/waitingroom && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/controller ./cmd/controller

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
COPY --from=build /out/controller /controller
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/server"]
