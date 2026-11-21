FROM ghcr.io/kukymbr/goose-docker:3.22.0
COPY migrations /migrations
USER 65532:65532
