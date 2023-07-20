DOCKER_TAG=cr7258/sample-controller:controller-runtime
APP = sample-controller-webhook-server
NAMESPACE = default

build:
	env GOOS=linux GOARCH=amd64 go build ./controller.go
docker-build: build
	docker build -t $(DOCKER_TAG) ./
docker-push: docker-build
	docker push $(DOCKER_TAG)
deploy:
	kubectl create configmap sample-controller-webhook-crt --from-file=./certs/tls.crt --from-file=./certs/tls.key
	kubectl apply -f ./foo.yaml
	kubectl apply -f ./deploy.yaml
	kubectl apply -f ./webhook.yaml
cert:
	@./ssl.sh $(APP) $(NAMESPACE)
clean:
	kubectl delete configmap sample-controller-webhook-crt
	kubectl delete -f ./deploy.yaml
	kubectl delete -f ./webhook.yaml
clean-cert:
	rm -rf certs/*
.PHONY: build docker-build docker-push deploy cert clean clean-cert