# Two stages: compile in a full Go image, then copy only the binaries into a
# minimal one. The final image has no shell, package manager or compiler, just
# our two programs, CA certificates (the scrapers call HTTPS APIs) and time
# zone data. Less in the image means less to patch and less to attack.

FROM golang:1.27-alpine AS build
WORKDIR /src

# Download modules before copying the source, so this layer stays cached until
# go.mod or go.sum change, not on every code edit.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 builds fully static binaries that don't need a C library in
# the final image. -trimpath and -s -w drop local paths and debug symbols.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scraper ./cmd/scraper

# distroless/static runs as a non-root user with the :nonroot tag.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /out/scraper ./
# The scraper reads sources.yaml from its working directory by default.
COPY sources.yaml ./

EXPOSE 8080
# The API is the default; run the scraper with: docker compose run --rm scraper
CMD ["/app/api"]
