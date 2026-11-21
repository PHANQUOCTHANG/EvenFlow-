FROM golang:1.23 AS build
RUN go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
COPY --from=build /out/controller /controller
USER 65532:65532
ENTRYPOINT ["/server"]
