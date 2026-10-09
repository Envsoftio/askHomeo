FROM node:22-bookworm-slim AS build
WORKDIR /ui
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

FROM nginx:stable-alpine AS runtime
COPY --from=build /ui/dist /usr/share/nginx/html
COPY apps/frontend/nginx.conf.template /etc/nginx/templates/default.conf.template

FROM runtime AS production
COPY apps/frontend/nginx.prod.conf /etc/nginx/nginx.conf
USER 101:101
# Render only our template into writable tmpfs; skip the root-oriented entrypoint.
ENTRYPOINT ["/bin/sh", "-ec"]
CMD ["envsubst '${FRONTEND_BIND_ADDR} ${FRONTEND_PORT} ${API_PORT}' < /etc/nginx/nginx.conf > /tmp/nginx.conf; exec nginx -c /tmp/nginx.conf -g 'daemon off;'"]

FROM runtime AS development
