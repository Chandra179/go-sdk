NAMESPACE ?= dev
IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: minikube-start
minikube-start:
	./start-minikube.sh $(NAMESPACE)

apply:
	@if [ "$(word 2, $(MAKECMDGOALS))" = "" ]; then \
		echo "❌ Usage: make apply <yaml-file>"; \
		exit 1; \
	fi; \
	FILE=$(word 2, $(MAKECMDGOALS)); \
	echo "🚀 Applying $$FILE"; \
	kubectl apply -f $$FILE

restart:
	@if [ "$(word 2, $(MAKECMDGOALS))" = "" ]; then \
		echo "❌ Usage: make restart <deployment-name>"; \
		exit 1; \
	fi; \
	DEPLOY=$(word 2, $(MAKECMDGOALS)); \
	echo "🔄 Restarting $$DEPLOY in namespace dev"; \
	kubectl rollout restart deployment/$$DEPLOY -n dev; \
	echo "✅ Rollout done for $$DEPLOY"

# Prevent treating arguments as targets
%:
	@: