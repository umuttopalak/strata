# strata-server: renders SVGs for public GitHub repositories.
# Build from the repository root: docker build -f deploy/server/Dockerfile .
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/strata-server ./cmd/strata-server

FROM alpine:3
# git clones the repositories; ca-certificates lets it and the API
# clients speak HTTPS.
RUN apk add --no-cache git ca-certificates && adduser -D -u 10001 strata
USER strata
COPY --from=build /out/strata-server /usr/local/bin/strata-server
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["strata-server"]
