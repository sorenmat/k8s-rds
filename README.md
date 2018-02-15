# k8s-rds

[![Build Status](https://travis-ci.org/sorenmat/k8s-rds.svg?branch=master)](https://travis-ci.org/sorenmat/k8s-rds)

A Custom Resource Definition for provisioning AWS RDS databases.


## Assumptions

The node running the pod should have an instance profile that allows creation and deletion of RDS databases and Subnets.

## Building

`go build`

## Installing

You can start the the controller by applying `kubectl apply -f deploy/deployment.yaml`

## Deploying

When the controller is running in the cluster you can deploy/crete a new database by running `kubectl apply` on the following
file.

```yaml
apiVersion: k8s.io/v1
kind: Database
metadata:
  name: test-pgsql
  namespace: default
  spec:
    class: db.t2.micro
    engine: postgres
    dbname: test-pgsql
    name: test-pgsql
    password: mysupersecretPW
    username: postgres
```

After the deploy is done you should be able to see your database via `kubectl get databases`

```shell
NAME         AGE
test-pgsql   11h
```