FROM golang:1.27-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY migrations ./migrations
COPY packages/contracts ./packages/contracts
COPY services ./services
RUN test -n "$SERVICE" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./services/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
