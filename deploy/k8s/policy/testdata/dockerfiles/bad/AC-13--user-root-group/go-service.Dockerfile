FROM gcr.io/distroless/static-debian12
COPY server /server
USER 10001:0
ENTRYPOINT ["/server"]
