include secrets/values.mk

RELEASE:=c12345-release-test
NAMESPACE:=c12345-test
VALUES?=./values.yaml

example-restore:
	kubectl -n ${NAMESPACE} apply -f ./examples/oracle.yaml

example-restore-delete:
	kubectl -n ${NAMESPACE} delete -f ./examples/oracle.yaml

# example-create:
# 	kubectl -n ${NAMESPACE} apply -f ./examples/postgres.yaml

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
