#!/usr/bin/env bash
set -e

NAMESPACE=${1:-dev}
MINIKUBE_BIN=$(which minikube)

# Color codes for better output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}✓${NC} $1"; }
log_warn() { echo -e "${YELLOW}⚠${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; }

#############################################
# PHASE 0: PRE-FLIGHT CHECKS
#############################################
echo "🔍 Phase 0: Pre-flight checks..."

# Check if minikube is running
if ! minikube status >/dev/null 2>&1; then
  log_info "Starting Minikube..."
  minikube start
else
  log_info "Minikube already running"
fi

#############################################
# PHASE 1: INGRESS SETUP
#############################################
echo ""
echo "🌐 Phase 1: Setting up ingress..."

minikube addons enable ingress

log_info "Waiting for ingress controller deployment..."
kubectl wait --namespace ingress-nginx \
  --for=condition=available deployment/ingress-nginx-controller \
  --timeout=180s

log_info "Waiting for ingress controller pods..."
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

# Wait for webhook to be fully ready
log_info "Waiting for admission webhook to stabilize..."
sleep 20

# Verify webhook endpoint
MAX_WEBHOOK_ATTEMPTS=15
WEBHOOK_ATTEMPT=0
while [ $WEBHOOK_ATTEMPT -lt $MAX_WEBHOOK_ATTEMPTS ]; do
  if kubectl get endpoints ingress-nginx-controller-admission -n ingress-nginx -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null | grep -q .; then
    log_info "Ingress webhook is ready"
    break
  fi
  WEBHOOK_ATTEMPT=$((WEBHOOK_ATTEMPT + 1))
  sleep 2
done

if [ $WEBHOOK_ATTEMPT -eq $MAX_WEBHOOK_ATTEMPTS ]; then
  log_warn "Webhook not ready, deleting ValidatingWebhookConfiguration..."
  kubectl delete validatingwebhookconfiguration ingress-nginx-admission --ignore-not-found
  sleep 5
fi

#############################################
# PHASE 2: CREATE NAMESPACES
#############################################
echo ""
echo "📁 Phase 2: Creating namespaces..."
kubectl apply -f namespace.yaml
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -

#############################################
# PHASE 3: INSTALL/VERIFY ARGOCD
#############################################
echo ""
echo "🔧 Phase 3: Setting up ArgoCD..."

if kubectl get deployment argocd-server -n argocd &>/dev/null; then
  log_info "ArgoCD already installed"
else
  log_info "Installing ArgoCD..."
  kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
fi

log_info "Waiting for ArgoCD to be ready..."
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=argocd-server \
  -n argocd \
  --timeout=300s

# Wait for CRDs to be ready
sleep 15

# Ensure default project exists
if ! kubectl get appproject -n argocd default &>/dev/null; then
  log_warn "Creating default ArgoCD project..."
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
fi

#############################################
# PHASE 4: APPLY OBSERVABILITY STACK
#############################################
echo ""
echo "📊 Phase 4: Deploying observability stack..."

# Apply configs first
kubectl apply -f prometheus-config.yaml
kubectl apply -f grafana-config.yaml
kubectl apply -f alloy-config.yaml

# Apply deployments
kubectl apply -f loki-deployment.yaml
kubectl apply -f prometheus-deployment.yaml
kubectl apply -f grafana-deployment.yaml
kubectl apply -f jaeger-deployment.yaml
kubectl apply -f alloy-deployment.yaml

log_info "Waiting for observability pods to be ready..."
kubectl wait --for=condition=ready pod -l app=prometheus -n observability --timeout=120s || log_warn "Prometheus not ready yet"
kubectl wait --for=condition=ready pod -l app=grafana -n observability --timeout=120s || log_warn "Grafana not ready yet"
kubectl wait --for=condition=ready pod -l app=loki -n observability --timeout=120s || log_warn "Loki not ready yet"

#############################################
# PHASE 5: APPLY APPLICATION
#############################################
echo ""
echo "🚀 Phase 5: Deploying application..."
kubectl apply -f app.yaml

log_info "Waiting for app pods to be ready..."
kubectl wait --for=condition=ready pod -l app=my-app -n "$NAMESPACE" --timeout=120s || log_warn "App pods not ready yet"

# Wait for services to have endpoints
log_info "Waiting for service endpoints..."
sleep 10

#############################################
# PHASE 6: APPLY INGRESS (AFTER SERVICES ARE READY)
#############################################
echo ""
echo "🌐 Phase 6: Configuring ingress..."

# Delete existing ingress to avoid conflicts
kubectl delete ingress observability-ingress -n observability --ignore-not-found 2>/dev/null
kubectl delete ingress app-ingress -n "$NAMESPACE" --ignore-not-found 2>/dev/null

# Apply ingress with retry
MAX_RETRIES=5
RETRY=0
SUCCESS=false

while [ $RETRY -lt $MAX_RETRIES ] && [ "$SUCCESS" = false ]; do
  if kubectl apply -f ingress.yaml 2>/tmp/ingress_error.log; then
    log_info "Ingress applied successfully"
    SUCCESS=true
  else
    RETRY=$((RETRY + 1))
    if [ $RETRY -lt $MAX_RETRIES ]; then
      log_warn "Ingress apply attempt $RETRY failed, retrying..."
      kubectl delete validatingwebhookconfiguration ingress-nginx-admission --ignore-not-found
      sleep 5
    else
      log_error "Failed to apply ingress after $MAX_RETRIES attempts"
      cat /tmp/ingress_error.log
    fi
  fi
done

# Wait for ingress to be ready
log_info "Waiting for ingress to be assigned address..."
sleep 15

#############################################
# PHASE 7: START MINIKUBE TUNNEL FIRST
#############################################
echo ""
echo "🔗 Phase 7: Starting minikube tunnel..."
echo "   This needs to run in a separate terminal with sudo"

# Check if tunnel is already running
if pgrep -f "minikube tunnel" > /dev/null; then
  log_info "Minikube tunnel already running"
else
  log_warn "Starting minikube tunnel in new terminal..."
  gnome-terminal -- bash -c "echo 'Starting Minikube Tunnel...'; sudo -E $MINIKUBE_BIN tunnel; exec bash" 2>/dev/null || {
    log_error "Could not open terminal. Run this manually:"
    echo "   sudo minikube tunnel"
    echo ""
    read -p "Press ENTER after starting minikube tunnel manually..."
  }
  
  # Wait for tunnel to establish
  log_info "Waiting for tunnel to establish..."
  sleep 10
fi

#############################################
# PHASE 8: VERIFY HOSTS FILE
#############################################
echo ""
echo "📝 Phase 8: Verifying /etc/hosts..."
MINIKUBE_IP=$(minikube ip)
echo "   Minikube IP: $MINIKUBE_IP"

REQUIRED_HOSTS=("grafana.local" "prometheus.local" "jaeger.local" "alloy.local" "app.local")
MISSING_HOSTS=()

for host in "${REQUIRED_HOSTS[@]}"; do
  if ! grep -q "$host" /etc/hosts 2>/dev/null; then
    MISSING_HOSTS+=("$host")
  fi
done

if [ ${#MISSING_HOSTS[@]} -gt 0 ]; then
  log_warn "Missing hosts entries. Add these to /etc/hosts:"
  echo ""
  echo "   sudo nano /etc/hosts"
  echo ""
  for host in "${MISSING_HOSTS[@]}"; do
    echo "   $MINIKUBE_IP $host"
  done
  echo ""
  read -p "Press ENTER after updating /etc/hosts..."
else
  log_info "All required hosts entries present"
fi

#############################################
# PHASE 9: REGISTER ARGOCD APPLICATION
#############################################
echo ""
echo "🎯 Phase 9: Registering ArgoCD application..."
kubectl apply -f argocd.yaml

#############################################
# PHASE 10: PORT FORWARDING
#############################################
echo ""
echo "🔌 Phase 10: Setting up port-forwards..."

# Kill existing port-forwards
pkill -f "kubectl port-forward.*$NAMESPACE.*8080" 2>/dev/null || true
pkill -f "kubectl port-forward.*argocd.*8081" 2>/dev/null || true

# Wait a bit for cleanup
sleep 2

# Start port-forwards with better error handling
log_info "Starting app port-forward..."
nohup kubectl port-forward svc/my-app -n "$NAMESPACE" 8080:8080 >/tmp/my-app.log 2>&1 &
APP_PF_PID=$!

log_info "Starting ArgoCD port-forward..."
nohup kubectl port-forward svc/argocd-server -n argocd 8081:443 >/tmp/argocd.log 2>&1 &
ARGOCD_PF_PID=$!

# Verify port-forwards are working
sleep 3
if ps -p $APP_PF_PID > /dev/null; then
  log_info "App port-forward running (PID: $APP_PF_PID)"
else
  log_warn "App port-forward failed. Check /tmp/my-app.log"
fi

if ps -p $ARGOCD_PF_PID > /dev/null; then
  log_info "ArgoCD port-forward running (PID: $ARGOCD_PF_PID)"
else
  log_warn "ArgoCD port-forward failed. Check /tmp/argocd.log"
fi

#############################################
# PHASE 11: GET ARGOCD PASSWORD
#############################################
echo ""
echo "🔑 ArgoCD Credentials:"
echo "   Username: admin"
echo -n "   Password: "
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "(retrieving...)"
echo ""

#############################################
# PHASE 12: HEALTH CHECKS
#############################################
echo ""
echo "🏥 Phase 12: Running health checks..."

# Check if services are responding
check_service() {
  local url=$1
  local name=$2
  if curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "$url" | grep -q "200\|302\|401"; then
    log_info "$name is responding"
    return 0
  else
    log_warn "$name is not responding yet"
    return 1
  fi
}

sleep 5

log_info "Checking service health (this may take a moment)..."
check_service "http://localhost:8080/health" "App" || true
check_service "http://$MINIKUBE_IP" "Ingress" || true

#############################################
# PHASE 13: OPEN BROWSERS
#############################################
echo ""
read -p "Open services in browser? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  log_info "Opening services in browser..."
  sleep 2
  
  xdg-open http://grafana.local 2>/dev/null &
  sleep 1
  xdg-open http://prometheus.local 2>/dev/null &
  sleep 1
  xdg-open http://jaeger.local 2>/dev/null &
  sleep 1
  xdg-open https://localhost:8081 2>/dev/null &
  sleep 1
  xdg-open http://app.local 2>/dev/null &
fi

#############################################
# SUMMARY
#############################################
echo ""
echo "✅ Setup complete!"
echo ""
echo "📋 Service URLs:"
echo "   • Grafana:    http://grafana.local (admin/admin)"
echo "   • Prometheus: http://prometheus.local"
echo "   • Jaeger:     http://jaeger.local"
echo "   • Alloy:      http://alloy.local"
echo "   • App:        http://app.local"
echo "   • ArgoCD:     https://localhost:8081"
echo ""
echo "🔍 Troubleshooting Commands:"
echo "   kubectl get pods -n $NAMESPACE"
echo "   kubectl get pods -n observability"
echo "   kubectl get pods -n argocd"
echo "   kubectl get ingress -A"
echo "   minikube service list"
echo ""
echo "🔄 If services aren't responding:"
echo "   1. Check minikube tunnel is running: pgrep -f 'minikube tunnel'"
echo "   2. Check /etc/hosts has correct entries"
echo "   3. Wait 30-60s for services to fully initialize"
echo "   4. Check logs: kubectl logs -n <namespace> <pod-name>"
echo ""