IMAGE     ?= httpserver:dev
CLUSTER   ?= http-lab
INGRESS_MANIFEST ?= https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml

.PHONY: image cluster ingress deploy-local vmstack scrape up logs clean

## build the server image
image:
	docker build -t $(IMAGE) .

## create the kind cluster (idempotent) with host ports 80/443 mapped
cluster:
	@kind get clusters | grep -qx $(CLUSTER) || \
		kind create cluster --name $(CLUSTER) --config deploy/kind-config.yaml

## install the kind ingress-nginx controller and wait for its admission webhook
ingress: cluster
	kubectl apply -f $(INGRESS_MANIFEST)
	# wait on the controller POD, not just the Deployment: the admission webhook
	# only accepts connections once the pod is Ready, otherwise applying the
	# Ingress fails with "connection refused" to the validating webhook.
	kubectl -n ingress-nginx wait --for=condition=ready pod \
		--selector=app.kubernetes.io/component=controller --timeout=180s

## build image, load it into kind, apply the manifests, make it reachable
deploy-local: image cluster ingress
	kind load docker-image $(IMAGE) --name $(CLUSTER)
	kubectl apply -f deploy/k8s/deployment.yaml \
		-f deploy/k8s/service.yaml \
		-f deploy/k8s/ingress.yaml
	kubectl rollout status deploy/httpserver --timeout=120s
	@echo ""
	@echo "reachable now:  curl http://localhost/healthz"
	@echo "fallback:       kubectl port-forward svc/httpserver 8080:80  then curl localhost:8080/healthz"

## install the VictoriaMetrics k8s stack (operator + vmsingle + vmagent)
vmstack: cluster
	helm repo add vm https://victoriametrics.github.io/helm-charts/ >/dev/null 2>&1 || true
	helm repo update >/dev/null
	helm upgrade --install vmstack vm/victoria-metrics-k8s-stack \
		-n monitoring --create-namespace \
		-f deploy/vmstack-values.yaml --wait --timeout 10m

## tell vmagent to scrape our Service
scrape:
	kubectl apply -f deploy/k8s/vmservicescrape.yaml

## everything: server + stack + scrape
up: deploy-local vmstack scrape

## tail the server logs
logs:
	kubectl logs -l app=httpserver --tail=50 -f

## delete the whole cluster
clean:
	kind delete cluster --name $(CLUSTER)
