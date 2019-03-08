include secrets/values.mk

RELEASE:=k8s-rds-release
NAMESPACE:=k8s-rds-test
VALUES?=./values.yaml

update-helm:
	helm dep update

template: update-helm
	@helm template . \
		-f ${VALUES} \
		--namespace=${NAMESPACE} \
		--name=${RELEASE} \
		--set image.pullSecret=${IMAGE_PULL_SECRET}

upgrade: update-helm
	@helm upgrade ${RELEASE} . \
		-f ${VALUES} \
		--namespace=${NAMESPACE} \
		--set image.pullSecret=${IMAGE_PULL_SECRET} \
		--debug \
		--install

install: update-helm
	@helm install . \
		-f ${VALUES} \
		--name=${RELEASE} \
		--namespace=${NAMESPACE} \
		--set image.pullSecret=${IMAGE_PULL_SECRET} \
		--timeout=9000 \
		--debug

delete:
	helm delete ${RELEASE} --timeout 600 --debug --tiller-connection-timeout 10

delete-release:
	helm delete ${RELEASE} --purge --debug

delete-nm:
	kubectl delete ns ${NAMESPACE} --timeout 10s --timeout=60s

.PHONY: update-helm template upgrade install delete delete-release delete-nm
