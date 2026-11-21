FROM gcr.io/distroless/static-debian12
COPY server /server
USER 0
ENTRYPOINT ["/server"]
