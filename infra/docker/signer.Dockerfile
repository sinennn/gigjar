FROM node:20-alpine
RUN corepack enable
WORKDIR /app
COPY services/signer/package.json services/signer/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile --prod
COPY services/signer/src ./src
USER node
EXPOSE 8081
CMD ["node", "src/server.js"]
