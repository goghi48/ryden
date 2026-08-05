FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ryden ./cmd/ryden

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ryden /app/ryden
EXPOSE 8080
ENTRYPOINT ["/app/ryden"]
