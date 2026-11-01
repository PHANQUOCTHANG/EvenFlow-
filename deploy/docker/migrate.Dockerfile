# syntax=docker/dockerfile:1
# Image cho Job goose tren k8s (eventflow-migrate). Compose van mount ../../migrations vao image
# goc; o k8s khong co bind mount nen migrations phai nam trong image.
#   docker build -f deploy/docker/migrate.Dockerfile -t eventflow-migrate .
# Entrypoint cua image goc chay `goose -dir=/migrations ${GOOSE_COMMAND:-up}` va doc
# GOOSE_DRIVER / GOOSE_DBSTRING tu env. goose `up` khong ghi file nao, nen migrations giu owner
# root (chi can doc duoc).
FROM ghcr.io/kukymbr/goose-docker:3.22.0
COPY migrations /migrations
# Dang so: runAsNonRoot cua kubelet chi doc Config.User. AC-13.
USER 65532:65532
