# syntax=docker/dockerfile:1

FROM node:22-alpine AS web-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.22-alpine AS api-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go env GOPROXY && go list -m all && CGO_ENABLED=0 GOOS=linux go build -mod=readonly -trimpath -ldflags="-s -w" -o /lumic .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=api-builder /lumic ./lumic
COPY --from=web-builder /app/frontend/dist ./public
EXPOSE 5500
ENTRYPOINT ["./lumic"]
