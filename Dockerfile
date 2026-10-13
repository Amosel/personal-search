FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/personal-search-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/personal-search-server /personal-search-server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/personal-search-server"]
