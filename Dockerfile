# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.27 AS build
WORKDIR /src
# cache modules first (std-lib only today, but keeps builds fast if deps appear)
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# static binary, no libc, so it runs on distroless/scratch
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/httpserver ./cmd/httpserver

# ---- run stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/httpserver /httpserver
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/httpserver"]
