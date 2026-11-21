FROM golang:1.23 AS build
USER 65532:65532
RUN go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
ENTRYPOINT ["/server"]
