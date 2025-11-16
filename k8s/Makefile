# Kubernetes
NAMESPACE ?= dev

########################################
# DOCKER IMAGE WORKFLOW
########################################

IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

########################################
# 1️⃣ MINIKUBE SETUP
########################################

.PHONY: minikube-start
minikube-start:
	./start-minikube.sh $(NAMESPACE)

.PHONY: kubectl-install
kubectl-install:
	curl -LO "https://dl.k8s.io/release/$$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
	chmod +x kubectl
	sudo mv kubectl /usr/local/bin/
	kubectl version --client

.PHONY: cluster-info
cluster-info:
	kubectl get nodes
	minikube status

.PHONY: busybox
busybox:
	kubectl run -it --rm busybox --image=busybox -- sh

.PHONY: test-ping
test-ping:
	ping 192.168.1.100

.PHONY: test-redis
test-redis:
	nc -zv 192.168.1.100 6379

.PHONY: k8s-info
k8s-info:
	kubectl get pods -A
	kubectl get pods -n $(NAMESPACE)
	kubectl get namespaces

.PHONY: describe-all
describe-all:
	kubectl get pods -n $(NAMESPACE) -o name | xargs -I{} kubectl describe {} -n $(NAMESPACE)

.PHONY: ingress-status
ingress-status:
	kubectl get pods -n ingress-nginx

.PHONY: wait-observability
wait-observability:
	kubectl get pods -n observability -w

########################################
# 2️⃣ DEPLOYMENT (YOUR YAML FILES)
########################################

.PHONY: apply-all
apply-all:
	@for f in *.yaml; do \
		echo "Applying $$f"; \
		kubectl apply -f $$f; \
	done

########################################
# 4️⃣ ARGOCD SETUP
########################################

.PHONY: argocd-install
argocd-install:
	kubectl create namespace argocd || true
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

.PHONY: argocd-port
argocd-port:
	kubectl port-forward svc/argocd-server -n argocd 8081:443

.PHONY: app-port
app-port:
	kubectl port-forward svc/$(IMAGE) -n $(NAMESPACE) 8080:80

.PHONY: argocd-password
argocd-password:
	kubectl -n argocd get secret argocd-initial-admin-secret \
	 -o jsonpath="{.data.password}" | base64 -d; echo

########################################
# 6️⃣ CLEANUP
########################################

.PHONY: clean
clean:
	kubectl delete -f .
