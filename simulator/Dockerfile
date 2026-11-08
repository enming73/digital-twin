FROM golang:1.24.11-alpine AS build
WORKDIR /src
COPY simulator/go.mod simulator/go.sum ./
RUN go mod download
COPY simulator/ ./
RUN CGO_ENABLED=0 go build -o /simulator ./cmd/simulator
FROM alpine:3.22.2
COPY --from=build /simulator /usr/local/bin/simulator
COPY simulator/scenarios /scenarios
ENTRYPOINT ["simulator"]
