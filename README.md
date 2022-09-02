# k8s-rds

[![Build Status](https://travis-ci.org/sorenmat/k8s-rds.svg?branch=master)](https://travis-ci.org/sorenmat/k8s-rds)
[![Go Report Card](https://goreportcard.com/badge/github.com/sorenmat/k8s-rds)](https://goreportcard.com/report/github.com/sorenmat/k8s-rds)

A Custom Resource Definition for provisioning AWS RDS databases.

State: BETA - use with caution

## Assumptions

The node running the pod should have an instance profile that allows creation and deletion of RDS databases and Subnets.

The codes will search for the first node, and take the subnets from that node. And depending on wether or not your DB should be public, then filter them on that. If any subnets left it will attach the DB to that.

## Building

`go build`

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

## Running 
```
Kubernetes database provisioner

Usage:
  k8s-rds [flags]

Flags:
      --exclude-namespaces strings   list of namespaces to exclude. Mutually exclusive with --include-namespaces.
  -h, --help                         help for k8s-rds
      --include-namespaces strings   list of namespaces to include. Mutually exclusive with --exclude-namespaces.
      --provider string              Type of provider (aws, local) (default "aws")
      --repository string            Docker image repository, default is hub.docker.com)
```

The provider can be started in two modes:

**Local** - this will provision a docker image in the cluster, and providing a database that way

**AWS** - This will use the AWS API to create a RDS database

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
  namespace: default
spec:
  class: db.t4g.medium # type of the db instance
  engine: postgres # what engine to use postgres, mysql, aurora-postgresql etc.
  version: "14.3"
  dbname: pgsql # name of the initial created database
  name: pgsql # name of the database at the provider
  password: # link to database secret
    key: mykey # the key in the secret
    name: mysecret # the name of the secret
  username: postgres # Database username
  size: 20 # size in GB
  MaxAllocatedSize: 50 # size in GB
  backupretentionperiod: 10 # days to keep backup, 0 means diable
  deleteprotection: true # don't delete the database even though the object is delete in k8s
  encrypted: true # should the database be encrypted - always enable for sandbox and production
  iops: 1000 # number of iops
  multiaz: true # multi AZ support - always enable for sandbox and production
  storagetype: gp2 # type of the underlying storage
  tags: "key=value,key1=value1"
  provider: aws # Optional either aws or local, will override the value the operator was started with 
  
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

- [X] Local PostgreSQL support

- [ ] Cluster support

- [ ] Google Cloud SQL for PostgreSQL support



