# Minikube
```
install minikube
minikube start
minikube start --memory=3000 --cpus=2

// linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
kubectl version --client

// verify
kubectl get nodes
minikube status

// expected output
NAME       STATUS   ROLES           AGE   VERSION
minikube   Ready    control-plane   1m    v1.28.0

// launch pod terminal
kubectl run -it --rm busybox --image=busybox -- sh

// ping private network ip
ping 192.168.1.100
// expected output
PING 192.168.1.100 (192.168.1.100): 56 data bytes
64 bytes from 192.168.1.100: seq=0 ttl=64 time=0.123 ms
64 bytes from 192.168.1.100: seq=1 ttl=64 time=0.101 ms
64 bytes from 192.168.1.100: seq=2 ttl=64 time=0.102 ms
--- 192.168.1.100 ping statistics ---
3 packets transmitted, 3 packets received, 0% packet loss
round-trip min/avg/max = 0.101/0.108/0.123 ms

// test redis port
nc -zv 192.168.1.100 6379

// get pods
kubectl get pods -A
kubectl get pods -n dev

// get namespace
kubectl get namespaces

```

# Apply config
```
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/dev/configmap.yaml
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/dev/deployment.yaml
kubectl apply -f /home/jukebox/Work/go-sdk/k8s/dev/service.yaml

kubectl get pods -n dev -o name | xargs -I{} kubectl describe {} -n dev
```

# Pushing image
```
// rebuild
docker build -t my-app:latest .

// tag
docker tag my-app:latest c1789/my-app:latest

// push
docker push c1789/my-app:latest
```

# ArgoCD
```
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl port-forward svc/argocd-server -n argocd 8081:443

kubectl apply -f /home/jukebox/Work/go-sdk/k8s/dev/application.yaml
```