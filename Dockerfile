FROM alpine:latest
WORKDIR /app
COPY alpr_search .
ENTRYPOINT ["./alpr_search"]
