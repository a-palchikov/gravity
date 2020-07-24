# syntax=docker/dockerfile:1.1.7-experimental

FROM node:12.18.3-stretch-slim

# TODO(dima)
# RUN --mount=type=cache,sharing=locked,id=aptlib,target=/var/lib/apt \
#    --mount=type=cache,sharing=locked,id=aptcache,target=/var/cache/apt \
RUN apt-get update && apt-get install git -y

# prepare to install only package.json dependencies
RUN mkdir -p /app
COPY package*.json /app/

# copy the rest of the app files
WORKDIR /app

RUN --mount=type=cache,sharing=locked,id=npm,target=/.cache/npm \
    npm set audit false ;\
    npm install --silent

COPY . /app

# and build
RUN --mount=type=cache,sharing=locked,id=npm,target=/.cache/npm \
    npm run build

# tar up the resulting build
RUN tar -C /app/dist -czf /web-assets.tar.gz .
