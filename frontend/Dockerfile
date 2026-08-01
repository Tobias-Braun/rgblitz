FROM node:20-alpine AS builder

WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM node:20-alpine

WORKDIR /app
COPY --from=builder /src/.output /app/.output

ENV PORT=3000 \
    HOST=0.0.0.0

EXPOSE 3000

CMD ["node", ".output/server/index.mjs"]
