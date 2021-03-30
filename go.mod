module github.com/sorenmat/k8s-rds

go 1.16

require (
	github.com/aws/aws-sdk-go-v2 v1.3.0
	github.com/aws/aws-sdk-go-v2/config v1.1.3
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.2.0
	github.com/aws/aws-sdk-go-v2/service/rds v1.2.0
	github.com/ghodss/yaml v1.0.0
	github.com/golangci/golangci-lint v1.17.2-0.20190910081425-f312a0fc4e31
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pkg/errors v0.8.1
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.4.0
	github.com/xeipuuv/gojsonschema v1.2.0
	k8s.io/api v0.0.0-20190905160310-fb749d2f1064
	k8s.io/apiextensions-apiserver v0.0.0-20190906235842-a644246473f1
	k8s.io/apimachinery v0.0.0-20190831074630-461753078381
	k8s.io/client-go v0.0.0-20190906195228-67a413f31aea
)
