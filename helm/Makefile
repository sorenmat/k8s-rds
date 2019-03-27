include secrets/values.mk

RELEASE:=k8s-rds
NAMESPACE:=k8s-rds
VALUES?=./values.yaml
EXAMPLE_NAMESPACE:=c12345-test

example-oracle:
	kubectl -n ${EXAMPLE_NAMESPACE} apply -f ./examples/oracle.yaml

example-oracle-delete:
	kubectl -n ${EXAMPLE_NAMESPACE} delete -f ./examples/oracle.yaml

# example-postgres:
# 	kubectl -n ${EXAMPLE_NAMESPACE} apply -f ./examples/postgres.yaml

# example-postgres-delete:
# 	kubectl -n ${EXAMPLE_NAMESPACE} delete -f ./examples/postgres.yaml

template:
	@helm template . \
		-f ${VALUES} \
		--namespace=${NAMESPACE} \
		--name=${RELEASE} \
		--set secrets.aws_access_key_id=${AWS_ACCESS_KEY_ID} \
		--set secrets.aws_secret_access_key=${AWS_SECRET_ACCESS_KEY} \
		--set image.pullSecret=${IMAGE_PULL_SECRET}

upgrade:
	@helm upgrade ${RELEASE} . \
		-f ${VALUES} \
		--namespace=${NAMESPACE} \
		--set secrets.aws_access_key_id=${AWS_ACCESS_KEY_ID} \
		--set secrets.aws_secret_access_key=${AWS_SECRET_ACCESS_KEY} \
		--set image.pullSecret=${IMAGE_PULL_SECRET} \
		--debug \
		--install

install:
	@helm install . \
		-f ${VALUES} \
		--name=${RELEASE} \
		--namespace=${NAMESPACE} \
		--set secrets.aws_access_key_id=${AWS_ACCESS_KEY_ID} \
		--set secrets.aws_secret_access_key=${AWS_SECRET_ACCESS_KEY} \
		--set image.pullSecret=${IMAGE_PULL_SECRET} \
		--timeout=9000 \
		--debug

delete:
	helm delete ${RELEASE} --purge --timeout 600 --debug --tiller-connection-timeout 10

delete-nm:
	kubectl delete ns ${NAMESPACE} --timeout 10s --timeout=60s

.PHONY: template upgrade install delete delete-nm
