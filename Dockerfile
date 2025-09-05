FROM scratch
WORKDIR /app
COPY alpr_service .
ENTRYPOINT ["/app/alpr_service"]
