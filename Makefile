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

observability:
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
	helm repo update prometheus-community >/dev/null
	helm upgrade --install kps prometheus-community/kube-prometheus-stack -n monitoring --create-namespace \
	  -f observability/prometheus/values.yaml --wait --timeout 10m
	kubectl apply -f observability/prometheus/servicemonitor.yaml
	kubectl -n monitoring create configmap sentinel-red-dashboard \
	  --from-file=observability/grafana/red-dashboard.json --dry-run=client -o yaml \
	  | kubectl label --local -f - grafana_dashboard=1 -o yaml | kubectl apply -f -
