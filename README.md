# k8s-rds

[![Build Status](https://travis-ci.org/cloud104/k8s-rds.svg?branch=master)](https://travis-ci.org/sorenmat/k8s-rds)
[![Go Report Card](https://goreportcard.com/badge/github.com/cloud104/k8s-rds)](https://goreportcard.com/report/github.com/cloud104/k8s-rds)

A Custom Resource Definition for provisioning AWS RDS databases.

State: BETA - use with caution

## Assumptions

The node running the pod should have an instance profile that allows creation and deletion of RDS databases and Subnets.

The codes will search for the first node, and take the subnets from that node. And depending on wether or not your DB should be public, then filter them on that. If any subnets left it will attach the DB to that.

## Building

`CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o k8s-rds .`

## Installing

You can start the the controller by applying `kubectl apply -f deploy/deployment.yaml`

### RBAC deployment

To create ClusterRole and bindings, apply the following instead:

```shell
kubectl apply -f deploy/operator-cluster-role.yaml
kubectl apply -f deploy/operator-service-account.yaml
kubectl apply -f deploy/operator-cluster-role-binding.yaml
kubectl apply -f deploy/deployment-rbac.yaml
```

## Deploying

When the controller is running in the cluster you can deploy/create a new database by running `kubectl apply` on the following
file.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
type: Opaque
data:
  mykey: cGFzc3dvcmRvcnNvbWV0aGluZw==
---
apiVersion: k8s.io/v1
kind: Database
metadata:
  name: pgsql
spec:
  backupRetentionPeriod: 10 # days to keep backup, 0 means diable
  class: db.t2.medium # type of the db instance
  dbname: pgsql # name of the initial created database
  encrypted: true # should the database be encrypted
  engine: postgres # what engine to use postgres, mysql, aurora-postgresql etc.
  iops: 1000 # number of iops
  multiaz: true # multi AZ support
  name: pgsql # name of the database at the provider
  size: 10 # size in BG
  storageType: gp2 # type of the underlying storage
  username: postgres # Database username
  password: # link to database secret
    key: mykey # the key in the secret
    name: mysecret # the name of the secret
```

After the deploy is done you should be able to see your database via `kubectl get databases`

```shell
NAME         AGE
test-pgsql   11h
```

And on the AWS RDS page

![subnets](docs/subnet.png "DB instance subnets")

![instances](docs/instances.png "DB instance")

# TODO

- [X] Basic RDS support
- [] Cluster support
- [] Google Cloud SQL for PostgreSQL support
- [] Local PostgreSQL support
- [] Transform rds creation into a configurable cli
- [] Azure support
- [] Make it read from a VERSION file and log
- [] Tests

## TEST

- [] Parallel running
- [] Pass parameter group
- [] Get latest snapshot when restoring
  - [] On delete check if snapshot was done correctly
- [] Delete check snapshot
- [] Create/Restore postgres
- [] Create/Restore oracle
