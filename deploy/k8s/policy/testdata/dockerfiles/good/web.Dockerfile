FROM node:22-alpine AS build
RUN npm run build

FROM node:22-alpine
COPY --from=build /app /app
USER 1000
CMD ["node", "server.js"]
