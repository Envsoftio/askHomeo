FROM node:22-bookworm-slim AS build
WORKDIR /ui
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

FROM nginx:stable-alpine
COPY --from=build /ui/dist /usr/share/nginx/html
COPY apps/frontend/nginx.conf.template /etc/nginx/templates/default.conf.template
