# Manual Kubernetes Route Debugging Guide

> This guide shows the **manual kubectl commands** you would run to debug routing issues if **KubeRoute did not exist**.
>
> Compare these multi-command workflows against running a single `./kuberoute` command.

---

## Prerequisites

- A running Kubernetes cluster (kind, minikube, or real)
- kubectl configured and pointing to your cluster

---

## Setup Test Environment

```bash
# Create cluster (if needed)
kind create cluster --name kuberoute-test

# Create namespace
kubectl create namespace kuberoute-test

# Deploy a healthy stack
kubectl apply -f 01-healthy-stack.yaml

# Verify pods are running
kubectl wait --for=condition=ready pod -l app=healthy-app -n kuberoute-test --timeout=60s
```

---

## Scenario 1: Healthy Stack (Everything Works)

### With KubeRoute (1 command)
```bash
./kuberoute --namespace=kuberoute-test --path=/api/users
```

### Without KubeRoute (5+ commands)
```bash
# Step 1: Find the ingress that handles /api
kubectl get ingress -n kuberoute-test
kubectl describe ingress healthy-ingress -n kuberoute-test

# Step 2: Check the backend service
kubectl get svc -n kuberoute-test
kubectl describe svc healthy-svc -n kuberoute-test

# Step 3: Check pods matching the service selectors
kubectl get pods -n kuberoute-test -l app=healthy-app
kubectl describe pod -l app=healthy-app -n kuberoute-test

# Step 4: Verify pod readiness
kubectl get pods -n kuberoute-test -l app=healthy-app -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}'
```

**Time:** ~3-5 minutes  
**Mental load:** High — you must manually connect ingress rules → service name → selectors → pod labels

---

## Scenario 2: 404 — No Ingress Matches the Path

### With KubeRoute (1 command)
```bash
./kuberoute --namespace=kuberoute-test --path=/this-does-not-exist
```

### Without KubeRoute (manual process)
```bash
# Step 1: List all ingresses in the namespace
kubectl get ingress -n kuberoute-test

# Step 2: Inspect EACH ingress to find path rules
kubectl describe ingress healthy-ingress -n kuberoute-test
kubectl describe ingress broken-ingress -n kuberoute-test
kubectl describe ingress mismatched-ingress -n kuberoute-test
# ... repeat for every ingress

# Step 3: Mentally check if any path matches /this-does-not-exist
# You must read through all the output and compare paths yourself
```

**Common mistake:** Forgetting to check one ingress, or missing a subtle path prefix mismatch.

**Root cause:** The path `/this-does-not-exist` is not defined in any ingress rule.

**Fix:** Add the missing path to an ingress, or ask the user for the correct URL.

---

## Scenario 3: 502 — Service Does Not Exist

### Setup
```bash
kubectl apply -f 02-missing-service.yaml
```

### With KubeRoute (1 command)
```bash
./kuberoute --namespace=kuberoute-test --path=/broken
```

### Without KubeRoute (manual process)
```bash
# Step 1: Find the ingress for /broken
kubectl get ingress -n kuberoute-test
kubectl describe ingress broken-ingress -n kuberoute-test
# Output shows backend service: missing-svc

# Step 2: Try to find the service
kubectl get svc missing-svc -n kuberoute-test
# Error: services "missing-svc" not found

# Step 3: Check if service exists with a different name
kubectl get svc -n kuberoute-test

# Step 4: Check if the service was deleted or renamed
kubectl get events -n kuberoute-test --sort-by='.lastTimestamp'
```

**Root cause:** The ingress references `missing-svc`, but that service was never created or was deleted.

**Fix:** Either:
- Create the missing service `kubectl apply -f <service-yaml>`
- OR update the ingress to point to the correct service name

---

## Scenario 4: 503 — No Pods Match Service Labels

### Setup
```bash
kubectl apply -f 03-label-mismatch.yaml
```

### With KubeRoute (1 command)
```bash
./kuberoute --namespace=kuberoute-test --path=/mismatch
```

### Without KubeRoute (manual process)
```bash
# Step 1: Find the ingress for /mismatch
kubectl get ingress -n kuberoute-test
kubectl describe ingress mismatched-ingress -n kuberoute-test
# Backend service: mismatched-svc

# Step 2: Check the service
kubectl describe svc mismatched-svc -n kuberoute-test
# Selectors: app=mismatched-app

# Step 3: Look for pods with that label
kubectl get pods -n kuberoute-test -l app=mismatched-app
# Output: No resources found

# Step 4: Check ALL pods in the namespace to compare labels
kubectl get pods -n kuberoute-test --show-labels

# Step 5: Realize there's a typo — pods have app=healthy-app but service wants app=mismatched-app
```

**Root cause:** The service selector (`app=mismatched-app`) does not match any pod labels.

**Fix:** Align the labels. Either:
- Fix the service selector: `kubectl edit svc mismatched-svc -n kuberoute-test`
- OR fix the deployment/pod labels: `kubectl edit deployment <name> -n kuberoute-test`

---

## Scenario 5: 503 — Pods Exist But Not Ready

### Setup
```bash
kubectl apply -f 04-not-ready-pods.yaml
# Wait ~20 seconds for readiness probe to fail
sleep 20
```

### With KubeRoute (1 command)
```bash
./kuberoute --namespace=kuberoute-test --path=/notready
```

### Without KubeRoute (manual process)
```bash
# Step 1: Find the ingress for /notready
kubectl get ingress -n kuberoute-test
kubectl describe ingress notready-ingress -n kuberoute-test
# Backend service: notready-svc

# Step 2: Check the service
kubectl describe svc notready-svc -n kuberoute-test
# Selectors: app=notready-app

# Step 3: Find pods with that label
kubectl get pods -n kuberoute-test -l app=notready-app
# Output: pod is running but status shows 0/1 Ready

# Step 4: Deep dive into why it's not ready
kubectl describe pod -l app=notready-app -n kuberoute-test
# Readiness probe failed: Get "http://10.244.0.1:80/healthz-fake": 404 Not Found

# Step 5: Check pod logs for application errors
kubectl logs -l app=notready-app -n kuberoute-test

# Step 6: Check events for clues
kubectl get events -n kuberoute-test --field-selector involvedObject.name=<pod-name>
```

**Root cause:** The readiness probe checks `/healthz-fake`, but the nginx container only serves `/`. The pod will never become Ready.

**Fix:** Fix the readiness probe path to match an actual endpoint:
```bash
kubectl edit deployment notready-app -n kuberoute-test
# Change readinessProbe.httpGet.path from /healthz-fake to /
```

---

## Summary: Time Comparison

| Scenario | With KubeRoute | Without KubeRoute (kubectl) |
|---|---|---|
| Healthy stack | 5 seconds | 3-5 minutes |
| 404 | 5 seconds | 2-3 minutes |
| 502 | 5 seconds | 3-4 minutes |
| 503 (no pods) | 5 seconds | 4-5 minutes |
| 503 (not ready) | 5 seconds | 5-8 minutes |

**KubeRoute saves 3-8 minutes per incident** by automating the trace and highlighting the root cause instantly.

---

## Quick Reference: Essential kubectl Commands

```bash
# List all resources in a namespace
kubectl get all -n kuberoute-test

# Describe a specific resource
kubectl describe <resource> <name> -n kuberoute-test

# Get pod logs
kubectl logs <pod-name> -n kuberoute-test

# Watch pods in real-time
kubectl get pods -n kuberoute-test -w

# Check events
kubectl get events -n kuberoute-test --sort-by='.lastTimestamp'

# Port-forward to test a service locally
kubectl port-forward svc/<service-name> 8080:80 -n kuberoute-test
```

---

## Cleanup

```bash
# Delete test namespace
kubectl delete namespace kuberoute-test

# Delete kind cluster (if you created one)
kind delete cluster --name kuberoute-test
```
