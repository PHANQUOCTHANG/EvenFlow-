# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
# `go.work` khai `use` CA 9 module. O che do workspace Go phai doc duoc go.mod cua TAT CA
# module do, nen ban cu -- chi copy libs/go va ticketing -- fail ngay o lenh dau voi
# `go: cannot load module ../gateway listed in go.work file` (tai hien tai cho tu ban
# export sach). Image nay CHUA TUNG build duoc: Trivy matrix ticketing do tren moi push len
# main, va nhieu kha nang day cung la ly do E2E nightly do o `make up` (chua tai hien rieng).
#
# Chi copy go.mod cua cac module KHONG dung, khong copy ma nguon cua chung: copy ca
# `services/` se keo file chua track cua moi service (`.env`, `*.pem`) vao build stage, va
# sua bat ky service nao cung lam mat cache cua image nay. Danh sach phai khop `go.work`:
# them module vao go.work thi them mot dong o day.
#
# Khong dung GOWORK=off de ne: repo khong track go.sum nao, nen o che do module `go build`
# doi checksum ma khong co (`missing go.sum entry`, da thu).
COPY go.work ./
COPY services/gateway/go.mod services/gateway/
COPY services/identity/go.mod services/identity/
COPY services/event/go.mod services/event/
COPY services/waitingroom/go.mod services/waitingroom/
COPY services/payment/go.mod services/payment/
COPY services/antibot/go.mod services/antibot/
COPY services/notification/go.mod services/notification/
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
