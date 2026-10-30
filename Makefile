REGISTRY ?= ghcr.io/mahesh-sadupalli
TAG      ?= dev
CLUSTER  ?= sentinel

.PHONY: test cluster images load deploy observability up down

test:
	go test -race ./...

cluster:
	kind get clusters | grep -qx $(CLUSTER) || kind create cluster --config infra/kind/cluster.yaml

images:
	docker build --build-arg CMD=control-plane  -t $(REGISTRY)/control-plane:$(TAG) .
	docker build --build-arg CMD=services/hello -t $(REGISTRY)/hello:$(TAG) .

load: images
	kind load docker-image --name $(CLUSTER) $(REGISTRY)/control-plane:$(TAG) $(REGISTRY)/hello:$(TAG)

deploy: load
	kubectl apply -k infra/k8s
	kubectl -n sentinel-system rollout restart deployment/control-plane
	kubectl -n sentinel-system rollout status deployment/control-plane --timeout=120s

up: cluster deploy

down:
	kind delete cluster --name $(CLUSTER)
