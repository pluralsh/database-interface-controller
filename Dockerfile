FROM golang:1.18.2-alpine3.15 as builder

ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/controller-manager/main.go main.go
COPY pkg/ pkg/


# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o controller-manager main.go


FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=builder /workspace/controller-manager .
USER 65532:65532
ENTRYPOINT ["/controller-manager"]
