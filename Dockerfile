FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN go mod init prsrv || true \
 && go mod tidy \
 && CGO_ENABLED=0 GOOS=linux go build -o /prsrv ./cmd/app

FROM alpine:3.20
WORKDIR /srv
ENV ADDR=:8080
COPY --from=build /prsrv /usr/local/bin/prsrv
COPY migrations ./migrations
EXPOSE 8080
CMD ["/usr/local/bin/prsrv"]
