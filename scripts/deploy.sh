#!/usr/bin/env sh

docker build -t "cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER" .
docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD";
docker tag "cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER" cloud104/k8s-rds:latest
docker push cloud104/k8s-rds:latest
docker push "cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER"
