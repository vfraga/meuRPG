# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# devidp: the development-only OpenID Connect provider (backend/cmd/devidp)
# behind the local stack (compose.yaml) and the Playwright tests in CI.
#
# It signs anyone in without a password, so it lives in its own image, used
# only by compose.yaml. The production image, backend/Dockerfile, builds only
# cmd/api and cmd/migrate, and devidp itself refuses to start unless its
# issuer is on a loopback host (see the package comment of cmd/devidp).
#
# Build context: the repo root, like backend/Dockerfile, so both share the
# root .dockerignore (no test files, no node_modules). The base images are
# the same ones, pinned by the same digests; refresh them together.

# ---- build stage -----------------------------------------------------------
FROM golang:1.27.2-trixie@sha256:e58d6f83b3416618d8bcac2b3dde1b7f7e3c4a77d25e88637f8bbae81536c48d AS build

WORKDIR /src

# Same as backend/Dockerfile: no silent toolchain download, verified modules.
ENV GOTOOLCHAIN=local

COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY backend/ .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/devidp ./cmd/devidp

# ---- runtime stage ----------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/devidp /app/devidp

USER nonroot:nonroot
EXPOSE 9090

ENTRYPOINT ["/app/devidp"]
