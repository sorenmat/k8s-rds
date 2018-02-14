# k8s-rds

[![Build Status](https://travis-ci.org/sorenmat/k8s-rds.svg?branch=master)](https://travis-ci.org/sorenmat/k8s-rds)


A Custom Resource Definition for provisioning AWS RDS databases.

## Building

`go build`

## Deploying

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
