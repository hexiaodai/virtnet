FROM --platform=$BUILDPLATFORM golang:1.20.3 as build

ARG TARGETARCH

ENV GOOS=linux \
    CGO_ENABLED=0 \
    GO111MODULE=on
# https://api.github.com/repos/containernetworking/plugins/releases/latest
ENV CNI_VERSION=v1.4.0

WORKDIR /virtnet
COPY . .

RUN go build -o virtnet-ipam ./cmd/ipam

RUN go build -o virtnet-coordinator ./cmd/coordinator

RUN mkdir -p /src/cni/bin && \
    curl -L -O https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-${TARGETARCH}-${CNI_VERSION}.tgz && \
    tar -xvf cni-plugins-linux-${TARGETARCH}-${CNI_VERSION}.tgz -C /src/cni/bin/ && \
    echo "save cni-plguins: ${CNI_VERSION} done"

# ===== release image =========

FROM --platform=$BUILDPLATFORM alpine:3.16

WORKDIR /
COPY --from=build /virtnet/virtnet-ipam /usr/plugins/cni/virtnet-ipam
COPY --from=build /virtnet/virtnet-coordinator /usr/plugins/cni/virtnet-coordinator

COPY --from=build /src/cni/bin/ /usr/plugins/cni

ADD ./tools/docker/virtnet-plugins/entrypoint.sh /
