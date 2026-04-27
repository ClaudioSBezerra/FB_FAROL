# Build Stage — cache bust: 2026-04-13-smartpick
FROM node:18-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
ARG VITE_APP_MODULE=smartpick
ENV VITE_APP_MODULE=$VITE_APP_MODULE
RUN npm run build && \
    echo "─── dist contents after build ───" && \
    ls -la dist/ && \
    echo "─── dist/assets ───" && \
    ls -la dist/assets/

# Serve Stage
FROM nginx:stable-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Verifica se os arquivos chegaram; falha o build se index.html não existir
RUN test -f /usr/share/nginx/html/index.html && \
    echo "─── served files ───" && \
    ls -la /usr/share/nginx/html/ && \
    ls -la /usr/share/nginx/html/assets/ 2>/dev/null || echo "no assets dir"

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]