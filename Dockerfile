################################################################################
##                               BUILD ARGS                                   ##
################################################################################
ARG GOLANG_IMAGE=golang:1.14.6
ARG ALPINE_ARCH=amd64

ARG DISTROLESS=gcr.io/distroless/base


################################################################################
##                              BUILD STAGE                                   ##
################################################################################
# Build the manager as a statically compiled binary so it has no dependencies
# libc, muscl, etc.
FROM ${GOLANG_IMAGE} as builder

WORKDIR /build
COPY go.mod go.sum ./
COPY pkg/ pkg/
COPY cmd/ cmd/
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
ENV GOPROXY ${GOPROXY:-https://proxy.golang.org}

# Get dependancies - will also be cached if we won't change mod/sum
RUN go mod download -x
# Blockquote Impact on build caching ARG variables are not persisted into the built image as ENV variables are.
# However, ARG variables do impact the build cache in similar ways.
# If a Dockerfile defines an ARG variable whose value is different from a previous build, then a “cache miss” occurs upon its first usage, not its definition.
# In particular, all RUN instructions following an ARG instruction use the ARG variable implicitly (as an environment variable), thus can cause a cache miss.
# All predefined ARG variables are exempt from caching unless there is a matching ARG statement in the Dockerfile.
ARG VERSION=unknown
ARG GOPROXY

RUN go build -a -ldflags="-w -s -extldflags '-static' -X 'main.version=${VERSION}'" -o vcloud-cloud-controller-manager ./cmd/vcloud-cloud-controller-manager

################################################################################
##                               MAIN STAGE                                   ##
################################################################################
# Copy the manager into the distroless image.
FROM ${DISTROLESS}

MAINTAINER Niclas.Schad@plusserver.com

COPY --from=builder /build/vcloud-cloud-controller-manager /bin/vcloud-cloud-controller-manager
ENTRYPOINT [ "/bin/vcloud-cloud-controller-manager" ]
CMD [ "--help" ]
