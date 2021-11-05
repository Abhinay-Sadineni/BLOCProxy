# Simple MiCoProxy Sidecar Proxy

The code is based off this [repo](https://github.com/ymedialabs/ReverseProxy)

## Building the Proxy

```bash
sudo docker build -t ratnadeepb/micoproxy:latest .
sudo docker push ratnadeepb/micoproxy:latest
```

## Building the init-container

```bash
sudo docker build -t ratnadeepb/micoinit -f Dockerfile.init .
sudo docker push ratnadeepb/micoinit:latest
```

## Running the MiCo tool

```bash
kubectl apply -f deployment.yaml
```

## Ports

All incoming requests to the pod are routed to port `62081` and all outgoing requests are sent to port `62082`.