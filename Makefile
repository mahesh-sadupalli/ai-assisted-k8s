REGISTRY ?= ghcr.io/mahesh-sadupalli
TAG      ?= dev
CLUSTER  ?= sentinel

.PHONY: test cluster images load deploy observability up down eks-up eks-push eks-deploy eks-destroy

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

# --- EKS (M3). Costs money while it runs: `make eks-destroy` when done. ---
TF  := terraform -chdir=infra/terraform
ECR  = $(shell $(TF) output -raw ecr_registry)
EKS_TAG ?= $(shell git rev-parse --short HEAD)

eks-up:
	$(TF) init -backend-config=backend.hcl
	$(TF) apply
	eval "$$($(TF) output -raw kubeconfig_command)"   # runs after apply, not at make parse time

eks-push:
	aws ecr get-login-password --region $(shell $(TF) output -raw region) | docker login --username AWS --password-stdin $(ECR)
	docker build --platform linux/amd64 --build-arg CMD=control-plane  -t $(ECR)control-plane:$(EKS_TAG) .
	docker build --platform linux/amd64 --build-arg CMD=services/hello -t $(ECR)hello:$(EKS_TAG) .
	docker push $(ECR)control-plane:$(EKS_TAG)
	docker push $(ECR)hello:$(EKS_TAG)

eks-deploy:
	helm upgrade --install ingress-nginx ingress-nginx --repo https://kubernetes.github.io/ingress-nginx \
	  -n ingress-nginx --create-namespace \
	  --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb --wait
	kubectl kustomize infra/k8s-eks \
	  | sed -e 's#ghcr.io/mahesh-sadupalli/control-plane:dev#$(ECR)control-plane:$(EKS_TAG)#' \
	        -e 's#ALLOWED_REGISTRIES: .*#ALLOWED_REGISTRIES: $(ECR)#' \
	  | kubectl apply -f -
	kubectl -n sentinel-system rollout status deployment/control-plane --timeout=180s
	@echo "Ingress URL: http://$$(kubectl -n ingress-nginx get svc ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"

eks-destroy:
	-helm uninstall ingress-nginx -n ingress-nginx --wait   # removes the NLB before the VPC goes
	$(TF) destroy
