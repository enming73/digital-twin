FROM golang:1.24-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /twin-backend ./cmd/twin-backend

FROM alpine:3.21
WORKDIR /app
COPY --from=build /twin-backend /app/twin-backend
COPY contracts/event.schema.json /contracts/event.schema.json
ENV TWIN_SCHEMA_PATH=/contracts/event.schema.json
USER 65534:65534
ENTRYPOINT ["/app/twin-backend"]
