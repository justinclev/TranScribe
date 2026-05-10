# syntax=docker/dockerfile:1.7
# ─────────────────────────────────────────────────────────────────────────────
# Stage 1 — dependency cache
#   Separate layer so that `go mod download` is only re-run when go.mod/go.sum
#   change, not when source files change.
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS deps
WORKDIR /build

# Copy dependency manifests first (best-practice layer caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2 — test
#   Run the full test suite as part of the image build so that `docker build`
#   fails fast on broken tests. CI and local developers both get the same gate.
#
#   -race requires CGO and a C compiler.  Alpine provides gcc via build-base.
#   CGO_ENABLED stays 1 only inside this stage; the final binary is still CGO-
#   free (see stage 3).
# ─────────────────────────────────────────────────────────────────────────────
FROM deps AS test
# Install C toolchain required by the race detector.
RUN apk add --no-cache gcc musl-dev
COPY . .
# -race detects data races; -count=1 disables result caching
RUN CGO_ENABLED=1 go test -race -count=1 ./...

# ─────────────────────────────────────────────────────────────────────────────
# Stage 3 — build
#   Produces a single statically linked binary with no CGO dependencies so it
#   runs on the scratch final image.
# ─────────────────────────────────────────────────────────────────────────────
FROM deps AS build
COPY . .

# Build-time args let CI inject the version SHA automatically.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.buildDate=${BUILD_DATE}" \
    -o /out/transcribe-server \
    ./cmd/server

# ─────────────────────────────────────────────────────────────────────────────
# Stage 4 — final (distroless/static)
#   No shell, no package manager, no OS utilities. Attack surface is minimal.
#   Uses nonroot user (65532) from the distroless image — never runs as root.
# ─────────────────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS final

# OCI image labels (https://github.com/opencontainers/image-spec)
LABEL org.opencontainers.image.title="transcribe" \
      org.opencontainers.image.description="SOC2-compliant docker-compose → Terraform converter" \
      org.opencontainers.image.source="https://github.com/justinclev/transcribe" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/transcribe-server /transcribe-server

# distroless nonroot runs as UID 65532 by default; make it explicit.
USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/transcribe-server"]
