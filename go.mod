module github.com/sorenmat/k8s-rds

go 1.16

require (
	github.com/aws/aws-sdk-go-v2 v1.16.3
	github.com/aws/aws-sdk-go-v2/config v1.15.4
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.37.0
	github.com/aws/aws-sdk-go-v2/service/rds v1.21.0
	github.com/ghodss/yaml v1.0.0
	github.com/golangci/golangci-lint v1.45.2
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.4.0
	github.com/stretchr/testify v1.7.1
	github.com/xeipuuv/gojsonschema v1.2.0
	k8s.io/api v0.23.6
	k8s.io/apiextensions-apiserver v0.23.6
	k8s.io/apimachinery v0.23.6
	k8s.io/client-go v0.23.6
)
