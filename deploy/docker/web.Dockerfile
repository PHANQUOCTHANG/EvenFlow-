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
# Next INLINE moi bien NEXT_PUBLIC_* vao bundle luc build, nen no phai co mat o day chu khong
# phai luc chay. Dat `environment:` trong compose hay `env:` trong Deployment deu VO TAC DUNG.
# Khong truyen --build-arg thi API_BASE = "" va queue-client goi URL tuong doi tren chinh
# origin cua pod web -> deploy "thanh cong" voi ca hai probe xanh ma frontend khong goi duoc
# backend. Xem contracts.md C7. Truyen gia tri that trong CD la viec CAN NGUOI DUYET.
ARG NEXT_PUBLIC_API_BASE=""
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production

# `server.js` do output standalone sinh ra doc `process.env.HOSTNAME`. Docker dat bien do
# thanh container id, nen thieu dong nay thi Next bind vao IP eth0 va KHONG bind loopback ->
# healthcheck cua compose (`fetch('http://127.0.0.1:3000/...')`) nhan ECONNREFUSED va service
# `web` unhealthy vinh vien. Xem contracts.md C3.
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

# Next tu dang ky handler SIGTERM: no `server.close()` roi `process.exit(0)` NGAY. Khi do
# /api/readyz khong tra 503 duoc nua ma tra ECONNREFUSED, va nhanh 503 thanh code chet trong
# van hanh. Co nay tat handler cua Next de `src/instrumentation.ts` tu lo:
# setReady(false) -> cho 5s drain -> exit(0). Xem contracts.md C1.1b.
ENV NEXT_MANUAL_SIG_HANDLE=1
# Next standalone output: image nho va khoi dong nhanh -- quan trong vi pre-warm
# truoc gio mo ban khong duoc phu thuoc vao pod khoi dong cham.
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
# `USER 1000` chu khong `USER node` — cung mot user (uid 1000 = node trong
# node:22-alpine), nhung phai la DANG SO.
#
# Manifest dat `runAsNonRoot: true` va co y KHONG dat `runAsUser`. Khi do kubelet
# phai tu xac minh user cua image khac 0, ma no chi doc duoc field `Config.User`
# cua image — no KHONG doc /etc/passwd ben trong image de resolve ten. Gap mot
# chuoi ten, kubelet tu choi container bang `CreateContainerConfigError:
# container has runAsNonRoot and image has non-numeric user (node)`. Tuc pod fail
# 100% thoi gian chu khong phai chi khi co hoi quy.
#
# Dang so giu nguyen y dinh cua manifest: neu ai do bo dong nay di thi container
# chay bang root va `runAsNonRoot` se chan lai — hoi quy van lo ra.
USER 1000
EXPOSE 3000
CMD ["node", "server.js"]
