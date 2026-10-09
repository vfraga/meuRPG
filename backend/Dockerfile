# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# MeuRPG image: the Go API server plus the Angular web app it serves on the
# same origin (see docs/architecture.md#web-app-web). Build context: the
# repo root, because this needs both backend/ and web/. The Dockerfile
# itself stays under backend/ (that is still the Go module it builds), so
# every build passes -f explicitly:
#
#   docker build -f backend/Dockerfile \
#     --build-arg VERSION=v0.1.0 --build-arg COMMIT=$(git rev-parse HEAD) \
#     -t meurpg-api .
#
# All three base images are pinned by digest (the multi-platform index, so
# the same line works on amd64 and arm64). A tag can be moved to a different
# image; a digest cannot. Refresh them with:
#   docker buildx imagetools inspect <image:tag> --format '{{.Manifest.Digest}}'
# The Node major (22) must match .node-version, and the Go version
# (1.27.2) must match backend/go.mod: CI tests on the versions in those two
# files and ships what is pinned here. The `# syntax` line above is pinned
# by digest too, so the build does not depend on a moving frontend image.

# ---- build stage: compile the Angular app -----------------------------------
FROM node:22-trixie-slim@sha256:b26b04c123d9ff8ab646ceb18b9d75a1173acf64b9a401094b906d27b29338d4 AS web-build

WORKDIR /src

# package.json/package-lock.json first, in their own layer: it is only
# rebuilt when a dependency changes, not on every source change.
COPY web/package.json web/package-lock.json ./
# Never run install scripts from a third-party package (see CONTRIBUTING.md).
RUN --mount=type=cache,target=/root/.npm \
    npm ci --ignore-scripts

COPY web/ .
RUN npm run build

# ---- build stage: compile static binaries -----------------------------------
FROM golang:1.27.2-trixie@sha256:e58d6f83b3416618d8bcac2b3dde1b7f7e3c4a77d25e88637f8bbae81536c48d AS build

WORKDIR /src

# Never let the go command fetch another toolchain: if go.mod asks for a
# newer Go than this image has, the build fails instead of silently
# compiling with a toolchain nobody pinned.
ENV GOTOOLCHAIN=local

# Download modules first, in their own layer: it is only rebuilt when
# go.mod/go.sum change, not on every code change. `go mod verify` checks
# the downloaded modules against go.sum.
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY backend/ .

ARG VERSION=dev
ARG COMMIT=unknown

# CGO_ENABLED=0: pure Go, statically linked, so it runs on distroless/static
#   (no libc there).
# -trimpath: no local file paths inside the binary (reproducible builds).
# -s -w: strip debug symbols to make the binary smaller.
# -X: inject the build information into cmd/api's version/commit variables.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
      -o /out/migrate ./cmd/migrate

# An empty folder for uploaded images (BLOB_DIR), made here because the
# runtime image has no shell to run mkdir. See the COPY below.
RUN mkdir -p /out/meurpg/images

# ---- runtime stage: just the binaries and the web build ---------------------
# distroless/static has CA certificates and timezone data but no shell or
# package manager, which leaves very little to attack. The :nonroot variant
# runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/api /out/migrate /app/
# The licence and the third-party notices travel with the binary (Apache 2.0
# section 4; the SRD is CC BY 4.0, the fonts OFL), as the NOTICE file asks.
COPY LICENSE NOTICE /licenses/
COPY third_party/licenses/ /licenses/third_party/
# Matches config.DefaultWebDir; override with $WEB_DIR if it ever moves.
COPY --from=web-build /src/dist/web/browser /app/web
# The images folder, owned by nonroot (uid and gid 65532) so the API can
# write to it. It stays empty and unused unless BLOB_DIR points at it: the
# local stack mounts a volume here (deploy/local/compose.yaml), and a new
# volume takes this folder's owner. Production will store images in Cloud
# Storage instead (docs/operations.md).
COPY --from=build --chown=65532:65532 /out/meurpg/ /var/lib/meurpg/

# OCI image labels (https://github.com/opencontainers/image-spec/blob/main/annotations.md),
# from the same build args as the binary. Last, so a new commit only
# rebuilds this tiny layer, not the COPYs above.
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="meurpg-api" \
      org.opencontainers.image.source="https://github.com/PuraFome/meuRPG" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

USER nonroot:nonroot
EXPOSE 8080

# The API is the default. Run migrations with the same image:
#   docker run --rm -e DATABASE_URL=... --entrypoint /app/migrate <image> up
ENTRYPOINT ["/app/api"]
