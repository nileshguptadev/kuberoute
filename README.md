# KubeRoute

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> A zero-dependency, read-only Kubernetes CLI troubleshooting tool that traces ingress → service → pod routing paths and pinpoints why your traffic is failing.

---

## The Problem

During a Kubernetes incident, engineers often face **context fragmentation**. When users report a `404`, `502`, or `503` error, you're left bouncing across multiple contexts:
- `kubectl get ingress -n <namespace>`
- `kubectl get svc -n <namespace>`
- `kubectl get pods -n <namespace>`
- `kubectl describe pod <name>`

You mentally stitch these resources together while under pressure. **KubeRoute eliminates that friction** by performing the trace for you and rendering a beautiful, color-coded diagnostic tree in your terminal.

---

## Features

- 🔍 **Ingress-to-Pod Tracing**: Given a namespace and HTTP path, KubeRoute walks the entire routing chain
- 🌳 **Beautiful Terminal Trees**: Color-coded ASCII/Unicode output with 🟢🔴❌💡 symbols for instant visual diagnosis
- 🧠 **Smart Diagnostics**: Detects label mismatches, missing services, and failing readiness probes — and tells you *why*
- 🛡️ **Read-Only & Safe**: Only performs List/Get operations. Zero risk to your cluster.
- ⚡ **Zero Runtime Dependencies**: Single Go binary. Just build and run.

---

## Installation

### Pre-built Binaries (Recommended)

Download the latest release for your platform from [GitHub Releases](https://github.com/nileshguptadev/kuberoute/releases):

**Linux (amd64):**
```bash
curl -LO https://github.com/nileshguptadev/kuberoute/releases/latest/download/kuberoute-linux-amd64.tar.gz
tar -xzf kuberoute-linux-amd64.tar.gz
chmod +x kuberoute-linux-amd64
sudo mv kuberoute-linux-amd64 /usr/local/bin/kuberoute
```

**macOS (Intel):**
```bash
curl -LO https://github.com/nileshguptadev/kuberoute/releases/latest/download/kuberoute-darwin-amd64.tar.gz
tar -xzf kuberoute-darwin-amd64.tar.gz
chmod +x kuberoute-darwin-amd64
sudo mv kuberoute-darwin-amd64 /usr/local/bin/kuberoute
```

**macOS (Apple Silicon):**
```bash
curl -LO https://github.com/nileshguptadev/kuberoute/releases/latest/download/kuberoute-darwin-arm64.tar.gz
tar -xzf kuberoute-darwin-arm64.tar.gz
chmod +x kuberoute-darwin-arm64
sudo mv kuberoute-darwin-arm64 /usr/local/bin/kuberoute
```

**Windows:**
Download `kuberoute-windows-amd64.zip` from the releases page, extract it, and add `kuberoute.exe` to your PATH.

### From Source

```bash
# Clone the repository
git clone https://github.com/nileshguptadev/kuberoute.git
cd kuberoute

# Build the binary
go build -o kuberoute

# (Optional) Move to your PATH
mv kuberoute /usr/local/bin/
```

### Requirements

- A valid `~/.kube/config` file with access to your target cluster
- (Only for building from source) Go 1.21+

---

## Usage

```bash
# Trace the default path "/" in the "default" namespace
./kuberoute

# Trace a specific path in a specific namespace
./kuberoute --namespace=my-app --path=/api/v1/users

# Example output when everything is healthy
🔍 KubeRoute — Tracing path "/api" in namespace "staging"

🟢 Ingress Matched
├── Name: api-ingress
├── Namespace: staging
├── Matched Path: /api
├── Backend Service: api-service
└── Backend Port: 8080

🟢 Service Found
├── Name: api-service
├── Type: ClusterIP
└── Selectors:
    ├── app: api
    └── version: v2

🟢 Pods Found (3 matching)
├── 🟢 api-7d9f4b8c5-x1abc (Ready)
├── 🟢 api-7d9f4b8c5-y2def (Ready)
└── ✅ All 3 pods are passing readiness probes.
```

---

## Example: Troubleshooting a 404

```bash
./kuberoute --namespace=prod --path=/checkout
```

If no ingress matches the path, you'll see:

```
🔴 Ingress Not Found
├── Namespace: prod
├── Target Path: /checkout
└── Result: No ingress rule matches the requested path.
```

---

## Example: Troubleshooting a 502

If the ingress points to a service that no longer exists:

```
🟢 Ingress Matched
├── Name: web-ingress
├── Backend Service: old-web-service
└── Backend Port: 80

🔴 Service Error (502)
├── Service Name: old-web-service
└── Error: services "old-web-service" not found
```

---

## Example: Troubleshooting a 503 (No Pods Matching)

```
🟢 Service Found
├── Name: frontend
└── Selectors:
    └── app: frontend

❌ No Matching Pods
└── Result: 0 pods match the service selectors.

💡 Diagnostic Suggestion
   The Service selectors do not match any Pod labels.
   Common causes:
   • Deployment / Pod labels contain a typo or mismatch
   • The workload is deployed in a different namespace
   • The Deployment's label selector differs from the Pod template labels
   • The Service selector key or value is misspelled
```

---

## Flags

| Flag        | Default   | Description                           |
|-------------|-----------|---------------------------------------|
| `--namespace` | `default` | Kubernetes namespace to investigate   |
| `--path`      | `/`       | HTTP path to trace through ingresses  |

---

## How It Works

KubeRoute executes three sequential audit steps:

1. **Ingress Evaluation**: Fetches all ingresses in the namespace, loops through rules and HTTP paths, and performs a prefix match against your `--path`. Extracts the target `serviceName` and `servicePort`.
2. **Service Evaluation**: Fetches the matched Service, verifies it exists, and extracts the `spec.Selector` map.
3. **Pod Endpoint Audit**: Fetches all Pods in the namespace, filters them locally by checking if Pod labels match the Service selectors exactly, and evaluates their `Ready` condition.

---

## Contributing

Contributions are welcome! Please feel free to open an issue or submit a pull request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
