FROM alpine:3.4
RUN apk -Uuv add ca-certificates
COPY ./bin/drone-rancher-linux64 /bin/drone-rancher
ENTRYPOINT ["/bin/drone-rancher"]

