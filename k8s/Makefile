NAMESPACE ?= dev
IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: minikube-start
minikube-start:
	./start-minikube.sh $(NAMESPACE)

.PHONY: roll

roll:
	@if [ "$(word 2, $(MAKECMDGOALS))" = "" ] || [ "$(word 3, $(MAKECMDGOALS))" = "" ]; then \
		echo "❌ Usage: make roll <yaml-file> <deployment-name>"; \
		exit 1; \
	fi; \
	FILE=$(word 2, $(MAKECMDGOALS)); \
	DEPLOY=$(word 3, $(MAKECMDGOALS)); \
	echo "🚀 Applying $$FILE"; \
	kubectl apply -f $$FILE; \
	echo "🔄 Restarting $$DEPLOY in namespace dev"; \
	kubectl rollout restart deployment/$$DEPLOY -n dev; \
	echo "✅ Rollout done for $$DEPLOY"

# Prevent "app.yaml" and "my-app" from being treated as Make targets
%:
	@: