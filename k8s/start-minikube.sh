#!/usr/bin/env bash
set -e

NAMESPACE=${1:-dev}
MINIKUBE_BIN=$(which minikube)

echo "🚀 Starting Minikube..."
minikube start

echo "📦 Enabling ingress addon..."
minikube addons enable ingress

echo "⏳ Waiting for ingress controller to be ready..."
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

# Also wait for the admission webhook deployment
echo "⏳ Waiting for ingress admission webhook..."
kubectl wait --namespace ingress-nginx \
  --for=condition=available deployment/ingress-nginx-controller \
  --timeout=120s

# Give webhook extra time to fully initialize
echo "⏳ Allowing webhook to stabilize..."
sleep 15

# Verify webhook endpoint exists
if kubectl get endpoints ingress-nginx-controller-admission -n ingress-nginx &>/dev/null; then
  echo "✅ Ingress webhook endpoint ready"
else
  echo "⚠️  Webhook endpoint not found, may have issues with ingress"
fi

#############################################
# PHASE 1: CREATE NAMESPACES
#############################################
echo ""
echo "📁 Phase 1: Creating namespaces..."
kubectl apply -f namespace.yaml
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -

#############################################
# PHASE 2: INSTALL/VERIFY ARGOCD
#############################################
echo ""
echo "🔧 Phase 2: Checking ArgoCD..."

# Check if ArgoCD is already installed
if kubectl get deployment argocd-server -n argocd &>/dev/null; then
  echo "✅ ArgoCD already installed, skipping installation..."
  
  # Check if ArgoCD is healthy
  if kubectl get pods -n argocd -l app.kubernetes.io/name=argocd-server --field-selector=status.phase=Running &>/dev/null; then
    echo "✅ ArgoCD is running"
  else
    echo "⚠️  ArgoCD pods not ready, waiting..."
    kubectl wait --for=condition=ready pod \
      -l app.kubernetes.io/name=argocd-server \
      -n argocd \
      --timeout=60s || echo "⚠️  ArgoCD not ready, continuing anyway..."
  fi
else
  echo "📦 Installing ArgoCD..."
  kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

  echo "⏳ Waiting for ArgoCD to be ready (this may take 2-3 minutes)..."
  kubectl wait --for=condition=ready pod \
    -l app.kubernetes.io/name=argocd-server \
    -n argocd \
    --timeout=300s

  # Give ArgoCD time to initialize CRDs and create default project
  echo "⏳ Waiting for ArgoCD to initialize..."
  sleep 20
fi

# Verify default project exists
if ! kubectl get appproject -n argocd default &>/dev/null; then
  echo "⚠️  Default project not found, creating it..."
  cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: default
  namespace: argocd
spec:
  description: Default project
  sourceRepos:
    - '*'
  destinations:
    - namespace: '*'
      server: '*'
  clusterResourceWhitelist:
    - group: '*'
      kind: '*'
EOF
  sleep 5
else
  echo "✅ ArgoCD default project exists"
fi

#############################################
# PHASE 3: APPLY OBSERVABILITY STACK
#############################################
echo ""
echo "📊 Phase 3: Deploying observability stack..."

# Apply in order: configs first, then deployments
kubectl apply -f prometheus-config.yaml
kubectl apply -f grafana-config.yaml
kubectl apply -f alloy-config.yaml

kubectl apply -f loki-deployment.yaml
kubectl apply -f prometheus-deployment.yaml
kubectl apply -f grafana-deployment.yaml
kubectl apply -f jaeger-deployment.yaml
kubectl apply -f alloy-deployment.yaml

echo "⏳ Waiting for observability pods to start..."
sleep 10

#############################################
# PHASE 4: APPLY APPLICATION
#############################################
echo ""
echo "🚀 Phase 4: Deploying application..."
kubectl apply -f app.yaml

echo "⏳ Waiting for app pods to start..."
sleep 10

#############################################
# PHASE 5: APPLY INGRESS
#############################################
echo ""
echo "🌐 Phase 5: Configuring ingress..."

# Function to check if webhook is ready
check_webhook_ready() {
  kubectl get endpoints ingress-nginx-controller-admission -n ingress-nginx -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null | grep -q .
}

# Wait for ingress webhook to be fully ready
echo "⏳ Waiting for ingress admission webhook to be ready..."
MAX_ATTEMPTS=30
ATTEMPT=0

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if check_webhook_ready; then
    echo "✅ Webhook is ready"
    break
  fi
  echo "   Attempt $((ATTEMPT + 1))/$MAX_ATTEMPTS - Webhook not ready, waiting..."
  sleep 2
  ATTEMPT=$((ATTEMPT + 1))
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
  echo "⚠️  Webhook not ready after ${MAX_ATTEMPTS} attempts"
  echo "🔧 Attempting to fix by deleting ValidatingWebhookConfiguration..."
  kubectl delete validatingwebhookconfiguration ingress-nginx-admission --ignore-not-found
  sleep 3
fi

# Delete existing ingress if it exists (to avoid conflicts)
kubectl delete ingress observability-ingress -n observability --ignore-not-found 2>/dev/null
kubectl delete ingress app-ingress -n "$NAMESPACE" --ignore-not-found 2>/dev/null

# Try to apply ingress with retry logic
echo "📝 Applying ingress configuration..."
MAX_RETRIES=3
RETRY=0
SUCCESS=false

while [ $RETRY -lt $MAX_RETRIES ] && [ "$SUCCESS" = false ]; do
  if kubectl apply -f ingress.yaml 2>/tmp/ingress_error.log; then
    echo "✅ Ingress applied successfully"
    SUCCESS=true
  else
    RETRY=$((RETRY + 1))
    if [ $RETRY -lt $MAX_RETRIES ]; then
      echo "⚠️  Attempt $RETRY failed, retrying in 5 seconds..."
      cat /tmp/ingress_error.log
      
      # Try deleting the webhook config as a fix
      kubectl delete validatingwebhookconfiguration ingress-nginx-admission --ignore-not-found
      sleep 5
    else
      echo "❌ Failed to apply ingress after $MAX_RETRIES attempts"
      echo "📋 Error details:"
      cat /tmp/ingress_error.log
      echo ""
      echo "🔧 Manual fix: Run these commands after the script completes:"
      echo "   kubectl delete validatingwebhookconfiguration ingress-nginx-admission"
      echo "   kubectl apply -f ingress.yaml"
    fi
  fi
done

#############################################
# PHASE 6: APPLY ARGOCD APPLICATION
#############################################
echo ""
echo "🎯 Phase 6: Registering ArgoCD application..."
kubectl apply -f argocd.yaml

#############################################
# START MINIKUBE TUNNEL
#############################################
echo ""
echo "🔗 Starting minikube tunnel..."
gnome-terminal -- bash -c "sudo -E $MINIKUBE_BIN tunnel; exec bash" 2>/dev/null || {
  echo "⚠️  Could not open gnome-terminal. Run 'minikube tunnel' manually in another terminal."
}

#############################################
# VERIFY HOSTS FILE
#############################################
echo ""
echo "📝 Checking /etc/hosts configuration..."
MINIKUBE_IP=$(minikube ip)
echo "   Your Minikube IP: $MINIKUBE_IP"
echo ""
echo "   Add these entries to /etc/hosts (if not already added):"
echo "   sudo nano /etc/hosts"
echo ""
echo "   $MINIKUBE_IP grafana.local"
echo "   $MINIKUBE_IP prometheus.local"
echo "   $MINIKUBE_IP jaeger.local"
echo "   $MINIKUBE_IP alloy.local"
echo "   $MINIKUBE_IP app.local"
echo ""

#############################################
# PORT FORWARDING
#############################################
echo "🔌 Starting port-forwards..."

# Kill any existing port-forwards
pkill -f "kubectl port-forward.*$NAMESPACE.*8080" 2>/dev/null || true
pkill -f "kubectl port-forward.*argocd.*8081" 2>/dev/null || true

# Wait for services to be ready before port-forwarding
echo "⏳ Waiting for services to be ready..."
kubectl wait --for=condition=ready pod -l app=my-app -n "$NAMESPACE" --timeout=60s || echo "⚠️  App pods not ready yet"
sleep 5

nohup kubectl port-forward svc/my-app -n "$NAMESPACE" 8080:8080 >/tmp/my-app.log 2>&1 &
nohup kubectl port-forward svc/argocd-server -n argocd 8081:443 >/tmp/argocd.log 2>&1 &

echo "   App: http://localhost:8080"
echo "   ArgoCD: https://localhost:8081"

#############################################
# ARGOCD PASSWORD
#############################################
echo ""
echo "🔑 ArgoCD Credentials:"
echo "   Username: admin"
echo -n "   Password: "
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "(secret not ready yet)"
echo ""

#############################################
# OPEN BROWSER
#############################################
echo ""
echo "🌐 Opening services in browser..."
sleep 3

xdg-open http://grafana.local 2>/dev/null &
xdg-open http://prometheus.local 2>/dev/null &
xdg-open http://jaeger.local 2>/dev/null &
xdg-open http://alloy.local 2>/dev/null &
xdg-open https://localhost:8081 2>/dev/null &
xdg-open http://app.local 2>/dev/null &

#############################################
# SUMMARY
#############################################
echo ""
echo "✅ Setup complete!"
echo ""
echo "📋 Quick Status Check:"
echo "   kubectl get pods -n $NAMESPACE"
echo "   kubectl get pods -n observability"
echo "   kubectl get pods -n argocd"
echo ""
echo "🎉 All services should be accessible via:"
echo "   • Grafana:    http://grafana.local (admin/admin)"
echo "   • Prometheus: http://prometheus.local"
echo "   • Jaeger:     http://jaeger.local"
echo "   • Alloy:      http://alloy.local"
echo "   • App:        http://app.local"
echo "   • ArgoCD:     https://localhost:8081"
echo ""