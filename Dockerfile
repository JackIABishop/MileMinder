# syntax=docker/dockerfile:1

# --- Build stage ---------------------------------------------------------
# The SvelteKit SPA is already committed under internal/web/dist and embedded
# via go:embed, so a Go-only build produces a self-contained binary — no Node
# stage is needed here.
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads before copying the rest of the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static, CGO-free Linux binary so it runs on a scratch/distroless base.
RUN CGO_ENABLED=0 GOOS=linux go build -o /mileminder .

# --- Final stage ---------------------------------------------------------
# Distroless static: no shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /mileminder /mileminder

EXPOSE 8080

ENTRYPOINT ["/mileminder"]
CMD ["serve", "--hosted", "--data-dir", "/data", "--port", "8080"]
