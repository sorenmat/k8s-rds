#!/bin/bash
if [ "$TRAVIS_BRANCH" == "master" ]; then
    docker build -t sorenmat/k8s-rds:$TRAVIS_BUILD_NUMBER .
    docker tag sorenmat/k8s-rds:$TRAVIS_BUILD_NUMBER sorenmat/k8s-rds:latest
    docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD";
    docker push sorenmat/k8s-rds:$TRAVIS_BUILD_NUMBER
    docker push sorenmat/k8s-rds:latest
else
    echo "Skipping docker push since we are not running on master"
fi
