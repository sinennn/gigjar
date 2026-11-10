FROM golang:1.27-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY migrations ./migrations
COPY services ./services
RUN CGO_ENABLED=0 go build -trimpath -o /out/gigjar ./services/gigjar

FROM node:22-alpine AS signer
RUN corepack enable
WORKDIR /signer
COPY services/signer/package.json services/signer/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile --prod
COPY services/signer/src ./src

FROM node:22-alpine
WORKDIR /app
COPY --from=go /out/gigjar /app/gigjar
COPY --from=signer /signer /app/signer
COPY infra/docker/render-entrypoint.sh /app/entrypoint.sh
USER node
EXPOSE 10000
CMD ["/app/entrypoint.sh"]
