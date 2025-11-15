```bash
# ===============================
# 1️⃣ Minikube Setup
# ===============================

# Start minikube with default resources
minikube start

# Install kubectl (Linux)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
kubectl version --client

# Verify cluster
kubectl get nodes
minikube status
# Expected Output:
# NAME       STATUS   ROLES           AGE   VERSION
# minikube   Ready    control-plane   1m    v1.28.0

# Launch pod terminal (Test network)
kubectl run -it --rm busybox --image=busybox -- sh

# Ping private network IP
ping 192.168.1.100
# Expected Output:
# 64 bytes from 192.168.1.100: seq=0 ttl=64 time=0.123 ms
# 64 bytes from 192.168.1.100: seq=1 ttl=64 time=0.101 ms
# 64 bytes from 192.168.1.100: seq=2 ttl=64 time=0.102 ms
# --- 192.168.1.100 ping statistics ---
# 3 packets transmitted, 3 packets received, 0% packet loss

# Test Redis port
nc -zv 192.168.1.100 6379

# Inspect pods and namespaces
kubectl get pods -A
kubectl get pods -n dev
kubectl get namespaces

# Enable ingress
minikube addons enable ingress

# ===============================
# 2️⃣ Apply Kubernetes Configs
# ===============================
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/namespace.yaml

# Deploy Loki
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/loki-deployment.yaml

# Deploy Prometheus
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/prometheus-config.yaml
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/prometheus-deployment.yaml

# Deploy Grafana
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/grafana-config.yaml
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/grafana-deployment.yaml

# Tracing
kubectl apply -f k8s/observability/jaeger-deployment.yaml

# Alloy
kubectl apply -f k8s/observability/alloy-config.yaml
kubectl apply -f k8s/observability/alloy-deployment.yaml

# Ingress
kubectl apply -f k8s/observability/ingress.yaml

# App
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/app.yaml

# Describe pods (for debugging)
kubectl get pods -n dev -o name | xargs -I{} kubectl describe {} -n dev

# verify its running
kubectl get pods -n ingress-nginx

# wait pods to be ready
kubectl get pods -n observability -w

# ===============================
# 3️⃣ Docker Image Workflow
# ===============================

# Rebuild image
docker build -t my-app:latest .

# Tag image
docker tag my-app:latest c1789/my-app:latest

# Push image to DockerHub
docker push c1789/my-app:latest

# ===============================
# 4️⃣ ArgoCD Setup
# ===============================

# Install ArgoCD
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Port-forward ArgoCD server (access UI)
kubectl port-forward svc/argocd-server -n argocd 8081:443
kubectl port-forward svc/my-app -n dev 8080:80

# Get initial admin password
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d

# ===============================
# ✅ Optional Notes / Best Practices
# ===============================
# - Use namespaces to separate dev, staging, and prod.
# - Always check kubectl get pods -n <namespace> after applying manifests.
# - Use kubectl logs <pod> for debugging runtime issues.
# - Consider using kustomize or Helm for multi-environment setups.

# ===============================
# 🌐 Access the Services
# ===============================
minikube ip

# Replace <MINIKUBE_IP> with your actual Minikube IP
sudo nano /etc/hosts

192.168.49.2 grafana.local
192.168.49.2 prometheus.local
192.168.49.2 jaeger.local
192.168.49.2 alloy.local
192.168.49.2 app.local