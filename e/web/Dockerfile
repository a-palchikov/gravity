# Copyright 2021 Gravitational Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG NPM_VER=7.11.0

FROM node:12-alpine AS builder
ARG NPM_VER

RUN set -ex && apk --no-cache add git python2 make build-base

RUN set -ex && \
	npm install --silent -g npm@${NPM_VER} gulp && \
	npm set audit false

# prepare to install only package.json dependencies
RUN mkdir -p /app
#COPY package*.json /app/
COPY package.json /app/

# copy the rest of the app files
WORKDIR /app

RUN set -ex && npm install
COPY . /app
# and build
RUN npm run build
# tar up the resulting build
RUN tar -C /app/dist -czf /web-assets.tar.gz .

FROM scratch AS releaser
COPY --from=builder /app/dist /
