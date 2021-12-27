FROM golang:1.15.2-alpine3.12 as builder
WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY cmd/ cmd/
RUN CGO_ENABLED=0 go build -o launcher ./cmd/launcher


FROM hyperledger/fabric-peer:amd64-2.4.1
RUN mkdir -p /builders/golang/bin

COPY --from=builder /workspace/launcher  /builders/golang/bin/externalcc
RUN ln -s /builders/golang/bin/externalcc /builders/golang/bin/detect
RUN ln -s /builders/golang/bin/externalcc /builders/golang/bin/build
RUN ln -s /builders/golang/bin/externalcc /builders/golang/bin/release
RUN ln -s /builders/golang/bin/externalcc /builders/golang/bin/run

COPY images/fabric-peer/2.3.2/k8scc.yaml /builders/golang/bin/k8scc.yaml
