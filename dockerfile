FROM golang:1.26-alpine AS plugins-builder
WORKDIR /app/tunnelgate
RUN apk add --no-cache build-base make
COPY backend/ ./
RUN go mod tidy
RUN CGO_ENABLED=1 make features

FROM golang:1.26-alpine AS backend-builder
WORKDIR /app/tunnelgate
RUN apk add --no-cache build-base make
COPY backend/ ./
RUN go mod tidy
RUN CGO_ENABLED=1 go build -o build/server ./cmd/server

FROM nginx:1.29-alpine AS runtime
ENV ADMIN_PORT=9200 \
    APP_PORT=9280 \
    LOG_LEVEL=info \
    TUNNEL_GATE=docker \
    ADMIN_FEATURE_PLUGINS=build/features/admin/*.so \
    PUBLIC_FEATURE_PLUGINS=build/features/public/*.so
WORKDIR /app/tunnelgate
RUN apk add --no-cache libcap \
    && setcap 'cap_net_bind_service=+ep' /usr/sbin/nginx \
    && mkdir -p data build/features /usr/share/nginx/html /var/cache/nginx /run /var/log/nginx \
    && chown -R nginx:nginx /app/tunnelgate /usr/share/nginx/html /var/cache/nginx /run /var/log/nginx /etc/nginx/conf.d
COPY --from=backend-builder /app/tunnelgate/build/server ./build/server
COPY --from=plugins-builder /app/tunnelgate/build/features ./build/features
COPY nginx.plugin.conf.template /etc/nginx/templates/default.conf.template
COPY frontend/dist /usr/share/nginx/html
RUN chown -R nginx:nginx /app/tunnelgate /usr/share/nginx/html /etc/nginx/templates
USER nginx
EXPOSE 80 9280
VOLUME ["/app/tunnelgate/data"]
CMD ["sh", "-c", "envsubst '${ADMIN_PORT}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf; ADMIN_FEATURE_PLUGINS='build/features/admin/*.so' PUBLIC_FEATURE_PLUGINS='build/features/public/*.so' ./build/server & backend_pid=$!; trap 'kill $backend_pid 2>/dev/null; wait $backend_pid 2>/dev/null' TERM INT; nginx -g 'daemon off;' & nginx_pid=$!; wait $nginx_pid; nginx_status=$?; kill $backend_pid 2>/dev/null; wait $backend_pid 2>/dev/null; exit $nginx_status"]
