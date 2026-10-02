# syntax=docker/dockerfile:1
FROM node:22-alpine AS deps
WORKDIR /app
# Copy ca lockfile va dung `npm ci`: `npm install` khong co lockfile thi moi lan build
# co the giai dependency khac nhau -> image CI khac image local.
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY apps/web ./
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production
# Next standalone output: image nho va khoi dong nhanh -- quan trong vi pre-warm
# truoc gio mo ban khong duoc phu thuoc vao pod khoi dong cham.
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
