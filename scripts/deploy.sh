#!/bin/bash
if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then
    docker build -t cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER .
    docker tag cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER cloud104/k8s-rds:latest
    docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD";
    docker push cloud104/k8s-rds:$TRAVIS_BUILD_NUMBER
    docker push cloud104/k8s-rds:latest
else
    echo "Skipping docker push since we are not running on master"
fi
