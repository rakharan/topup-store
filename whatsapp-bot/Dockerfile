FROM node:20-alpine

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install --production

COPY . .

RUN addgroup -g 1001 appgroup && \
    adduser -u 1001 -G appgroup -D appuser && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 3001

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:3001/health || exit 1

CMD ["node", "index.js"]
