FROM --platform=$BUILDPLATFORM golang:1.20.3 as build
ENV GOOS=linux \
    CGO_ENABLED=0 \
    GO111MODULE=on
WORKDIR /virtnet
COPY . .
RUN go build -o virtnet-agent ./cmd/agent

FROM --platform=$BUILDPLATFORM alpine:3.16
COPY --from=build /virtnet/virtnet-agent /bin/virtnet-agent
RUN chmod go+x /bin/virtnet-agent
CMD ["virtnet-agent"]
