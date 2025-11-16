#!/usr/bin/env bash
set -e

NAMESPACE=${1:-dev}
MINIKUBE_BIN=$(which minikube)

echo "Starting Minikube..."
minikube start

echo "Enabling ingress addon..."
minikube addons enable ingress

echo "Waiting for ingress controller to be ready..."
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

#############################################
# APPLY-ALL (equivalent to your Makefile)
#############################################
echo "Applying all *.yaml files in current directory..."
for f in *.yaml; do
  echo "Applying $f"
  kubectl apply -f "$f"
done


#############################################
# START MINIKUBE TUNNEL
#############################################
echo "Starting minikube tunnel in a new terminal..."
gnome-terminal -- bash -c "sudo -E $MINIKUBE_BIN tunnel; exec bash"

# Replace <MINIKUBE_IP> with your actual Minikube IP
# sudo nano /etc/hosts

# 192.168.49.2 grafana.local
# 192.168.49.2 prometheus.local
# 192.168.49.2 jaeger.local
# 192.168.49.2 alloy.local
# 192.168.49.2 app.local

#############################################
# PORT FORWARDING
#############################################
echo "Starting port-forwards in background..."
nohup kubectl port-forward svc/my-app -n "$NAMESPACE" 8080:8080 >/tmp/my-app.log 2>&1 &
nohup kubectl port-forward svc/argocd-server -n argocd 8081:443 >/tmp/argocd.log 2>&1 &


#############################################
# OPEN BROWSER
#############################################
echo "Opening services in browser..."
xdg-open http://grafana.local &
xdg-open http://prometheus.local &
xdg-open http://jaeger.local &
xdg-open http://alloy.local &
xdg-open http://argocd.local &
xdg-open http://app.local &

echo "🎉 Done! Minikube running, YAML applied, tunnel active, port-forwards started."
