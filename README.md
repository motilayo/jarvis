# Jarvis: Distributed Command Execution on Kubernetes

Jarvis is a Kubernetes-native system for securely executing shell commands across cluster nodes using a custom controller and a gRPC agent deployed as a DaemonSet.

## Architecture
- **Controller**: Watches for `Command` custom resources and orchestrates command execution on selected nodes.
- **Agent**: Runs as a DaemonSet on every node, exposes a gRPC server to execute shell commands and return results.
- **CRDs**: Defines the `Command` resource for specifying commands and node selectors.

## Repository Structure
- `agent/`: gRPC agent source, Dockerfile, deployment manifests
- `controller/`: Kubernetes controller, CRDs, deployment manifests
- `kind/`: config to create a 3 node kind cluster

## Features
- Concurrent command execution on selected nodes
- Node selection via Kubernetes labels
- Result reporting via Kubernetes events
- Extensible via custom resources

## Usage
1. **Deploy the agent**:
   - Build and push the agent Docker image:
     ```sh
     cd agent
     make docker-build && make docker-push
     ```
   - Apply manifests:
     ```sh
     kubectl apply -f agent/deploy/
     ```
2. **Deploy the controller**:
   - Build and push the controller Docker image:
     ```sh
     cd controller
     make build
     make docker-build && make docker-push
     ```
   - Apply manifests:
     ```sh
     make install && make deploy
     ```
3. **Create a Command resource**:
   - Example:
     ```yaml
     apiVersion: jarvis.io/v1
     kind: Command
     metadata:
       name: sample-command
     spec:
       command: "hostname"
       selector:
         nodeSelectorTerms:
           - matchExpressions:
               - key: kubernetes.io/hostname
                 operator: In
                 values: ["node-1"]
     ```
   - Apply with:
     ```sh
     kubectl apply -f <your-command>.yaml
     ```

## Command Resource
`Command` CRDs live in the `jarvis.io/v1` API group. Each resource describes a shell command and optional node selector. The controller reconciles the CR, discovers matching nodes via EndpointSlices, and executes the command concurrently on every agent with a reachable IP.

- **Spec fields**:
  - `command` – required shell string executed via `/bin/sh -c` inside the agent (currently chrooted to `/host` to use node binaries).
  - `selector` – optional `NodeSelector`; omit to target all nodes.
  - `timeoutSeconds` – (planned) execution timeout per node.

Example CR (`controller/config/samples/v1_command.yaml`):
```yaml
apiVersion: jarvis.io/v1
kind: Command
metadata:
  name: command-sample
spec:
  command: ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10
  selector:
    nodeSelectorTerms:
      - matchExpressions:
          - key: kubernetes.io/os
            operator: In
            values: ["linux"]
```

## Event Flow
For every node targeted by a `Command`, the controller emits Kubernetes events in the same namespace as the CR. Events are keyed by `<command-name>-<node-name>` and capture both success and failure states.

Try the sample command to see events in action:

```sh
kubectl apply -f controller/config/samples/v1_command.yaml
```

Check that the `Command` resource exists in the `jarvis` namespace:

```sh
kubectl get command -n jarvis
```

Example:

```
NAME             AGE
command-sample   46h
```

Then inspect the per-node events directly with `kubectl`:

```sh
❯ kubectl get events -n jarvis --field-selector involvedObject.name=command-sample
LAST SEEN   TYPE     REASON                              OBJECT                   MESSAGE
11m         Normal   command-sample-kind-worker          command/command-sample   ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10...
11m         Normal   command-sample-kind-worker2         command/command-sample   ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10...
2m28s       Normal   command-sample-kind-worker2         command/command-sample   ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu...
2m28s       Normal   command-sample-kind-worker          command/command-sample   ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu...
2m28s       Normal   command-sample-kind-control-plane   command/command-sample   ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu...

```

Full event:

```yaml
apiVersion: v1
kind: Event
metadata:
  name: command-sample.1872cd8a3f1c4f44
  namespace: jarvis
type: Normal
reason: command-sample-kind-worker
message: |-
  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10
      PID COMMAND         %CPU %MEM
      211 kubelet          1.7  1.2
      105 containerd       0.7  1.3
     5401 manager          0.2  1.0
      450 kindnetd         0.0  0.4
      361 kube-proxy       0.0  0.5
     5306 containerd-shim  0.0  0.2
      264 containerd-shim  0.0  0.2
     5349 containerd-shim  0.0  0.2
      272 containerd-shim  0.0  0.2
```

Event is also published to the resource

```
❯ k describe command -n jarvis
Name:         command-sample
Namespace:    jarvis
Labels:       app.kubernetes.io/managed-by=kustomize
              app.kubernetes.io/name=controller
Annotations:  <none>
API Version:  jarvis.io/v1
Kind:         Command
Metadata:
  Creation Timestamp:  2025-10-27T01:36:11Z
  Finalizers:
    jarvis.io/finalizer
  Generation:        15
  Resource Version:  140790
  UID:               354e97c9-eb7a-4ccb-9c59-2d87666017d5
Spec:
  Command:  ps -eo pid,comm,%cpu,%mem --sort=-%cpu
Events:
  Type    Reason                      Age   From                Message
  ----    ------                      ----  ----                -------
  Normal  command-sample-kind-worker  13m   command-controller  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10
    PID COMMAND         %CPU %MEM
    211 kubelet          1.7  1.2
    105 containerd       0.7  1.3
   5401 manager          0.2  1.0
    450 kindnetd         0.0  0.4
    361 kube-proxy       0.0  0.5
   5306 containerd-shim  0.0  0.2
    264 containerd-shim  0.0  0.2
   5349 containerd-shim  0.0  0.2
    272 containerd-shim  0.0  0.2
  Normal  command-sample-kind-worker2  13m  command-controller  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu | head -n 10
    PID COMMAND         %CPU %MEM
    210 kubelet          1.6  1.2
    105 containerd       0.6  1.2
    452 kindnetd         0.0  0.4
    364 kube-proxy       0.0  0.5
    266 containerd-shim  0.0  0.2
   5144 containerd-shim  0.0  0.2
    275 containerd-shim  0.0  0.2
      1 systemd          0.0  0.1
     90 systemd-journal  0.0  0.0
  Normal  command-sample-kind-worker2  3m40s  command-controller  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu
    PID COMMAND         %CPU %MEM
    210 kubelet          1.6  1.2
    105 containerd       0.6  1.2
    452 kindnetd         0.0  0.4
    364 kube-proxy       0.0  0.5
    266 containerd-shim  0.0  0.2
   5144 containerd-shim  0.0  0.2
    275 containerd-shim  0.0  0.2
      1 systemd          0.0  0.1
     90 systemd-journal  0.0  0.0
   5194 jarvis-server    0.0  0.2
    317 pause            0.0  0.0
    324 pause            0.0  0.0
   5167 pause            0.0  0.0
  16233 chroot           0.0  0.1
  16234 sh               0.0  0.0
  16235 ps               0.0  0.0
  Normal  command-sample-kind-worker  3m40s  command-controller  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu
    PID COMMAND         %CPU %MEM
    211 kubelet          1.7  1.2
  16456 manager          0.8  0.9
    105 containerd       0.7  1.2
    450 kindnetd         0.0  0.4
    361 kube-proxy       0.0  0.5
    264 containerd-shim  0.0  0.2
   5349 containerd-shim  0.0  0.2
    272 containerd-shim  0.0  0.2
      1 systemd          0.0  0.1
     90 systemd-journal  0.0  0.0
   5438 jarvis-server    0.0  0.2
    315 pause            0.0  0.0
    322 pause            0.0  0.0
   5372 pause            0.0  0.0
  16404 containerd-shim  0.0  0.2
  16427 pause            0.0  0.0
  16482 chroot           0.0  0.1
  16483 sh               0.0  0.0
  16484 ps               0.0  0.0
  Normal  command-sample-kind-control-plane  3m40s  command-controller  ❯ ps -eo pid,comm,%cpu,%mem --sort=-%cpu
    PID COMMAND         %CPU %MEM
    548 kube-apiserver   5.4  6.8
    682 kubelet          3.4  1.5
    622 etcd             2.9  1.2
    521 kube-controller  2.1  1.8
    512 kube-scheduler   1.1  0.8
    105 containerd       0.8  1.2
   1541 coredns          0.2  0.5
   1550 coredns          0.2  0.5
    961 kindnetd         0.0  0.5
    871 kube-proxy       0.0  0.4
    299 containerd-shim  0.0  0.2
    773 containerd-shim  0.0  0.2
    268 containerd-shim  0.0  0.2
    802 containerd-shim  0.0  0.2
   1230 containerd-shim  0.0  0.2
   1232 containerd-shim  0.0  0.2
    281 containerd-shim  0.0  0.2
   1210 containerd-shim  0.0  0.2
    308 containerd-shim  0.0  0.2
  14076 containerd-shim  0.0  0.2
   1558 local-path-prov  0.0  0.3
      1 systemd          0.0  0.1
  14126 jarvis-server    0.0  0.2
     90 systemd-journal  0.0  0.0
    379 pause            0.0  0.0
    386 pause            0.0  0.0
    393 pause            0.0  0.0
    396 pause            0.0  0.0
    816 pause            0.0  0.0
    834 pause            0.0  0.0
   1290 pause            0.0  0.0
   1295 pause            0.0  0.0
   1303 pause            0.0  0.0
  14099 pause            0.0  0.0
  14200 chroot           0.0  0.1
  14201 sh               0.0  0.0
  14202 ps               0.0  0.0
```

This event-driven reporting makes it easy to audit multi-node execution without tailing logs; cluster operators can fetch the latest output with native Kubernetes tooling.

## Development
- Go modules for both agent and controller
- Run tests:
  ```sh
  cd controller
  make test
  ```
- Build binaries:
  ```sh
  cd agent && go build ./...
  cd controller && go build ./...
  ```
