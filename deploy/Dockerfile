# Image deployed to photobooth.apexvoid.net. Build context: the repository root (webpage branch).
#   docker build -f deploy/Dockerfile -t apexvoid-photobooth-webpage .
# Serves the static site on port 80; TLS and the domain are the central routing proxy's job.
FROM node:22-bookworm-slim AS build
WORKDIR /site
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown
LABEL org.opencontainers.image.title="apexvoid-photobooth-webpage" \
      org.opencontainers.image.description="ApexVoid Photobooth product website (static)" \
      org.opencontainers.image.source="https://github.com/st-mich43l/apexvoid-photobooth/tree/webpage" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$REVISION" \
      org.opencontainers.image.created="$CREATED"
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /site/dist /usr/share/nginx/html
EXPOSE 80
