# Support setting various labels on the final image
ARG COMMIT="7a7ed3988e41a08baef775571e157d6828a55397"
ARG VERSION="v0.6.1"
ARG BUILDNUM="1"

# Build Geth in a stock Go builder container
FROM golang:1.18-alpine as builder

RUN apk add --no-cache gcc musl-dev linux-headers git

ADD . /go-ethereum
RUN cd /go-ethereum && go run build/ci.go install ./cmd/geth

# Pull Geth into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /go-ethereum/build/bin/geth /usr/local/bin/

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["geth"]
CMD ["--maxpeers", \
     "100" , \
     "--ws", \
     "--ws.addr", \
     "0.0.0.0", \
     "--ws.port", \
     "8545", \
     "--ws.origins", \
     "136.25.196.81", \
     "--ws.rpcprefix" , \
     "/", \
     "--datadir", \
     "/data"]
# Add some metadata labels to help programatic image consumption
ARG COMMIT="7a7ed3988e41a08baef775571e157d6828a55397"
ARG VERSION="v0.6.1"
ARG BUILDNUM="1"

LABEL commit="$COMMIT" version="$VERSION" buildnum="$BUILDNUM"