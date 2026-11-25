# Image for the future deployment (WEB-2). Build context: the repository root (webpage branch).
#   docker build -f deploy/Dockerfile -t apexvoid-photobooth-webpage .
# Serves the static site on port 80; TLS and the domain are the central routing proxy's job.
FROM node:22-bookworm-slim AS build
WORKDIR /site
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /site/dist /usr/share/nginx/html
EXPOSE 80
