package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TraceRoute traces the Kubernetes routing path from Ingress -> Service -> Pod for a given namespace and targetPath.
func TraceRoute(clientset kubernetes.Interface, namespace string, targetPath string) error {
	ctx := context.Background()

	// ── Step 1: Ingress Evaluation ────────────────────────────────────────
	matchedIngress, matchedPath, matchedServiceName, matchedServicePort, found := findMatchingIngress(ctx, clientset, namespace, targetPath)
	if !found {
		printNotFoundTree(namespace, targetPath)
		return nil
	}

	printIngressTree(matchedIngress, matchedPath, matchedServiceName, matchedServicePort)

	// ── Step 2: Service Evaluation ────────────────────────────────────────
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, matchedServiceName, metav1.GetOptions{})
	if err != nil {
		printServiceErrorTree(matchedServiceName, err)
		return nil
	}

	printServiceTree(service)

	// ── Step 3: Pod Endpoint Audit ────────────────────────────────────────
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	matchedPods := filterPodsBySelector(pods.Items, service.Spec.Selector)
	printPodTree(matchedPods, service.Spec.Selector)

	return nil
}

func findMatchingIngress(ctx context.Context, clientset kubernetes.Interface, namespace, targetPath string) (*networkingv1.Ingress, string, string, int32, bool) {
	ingresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		color.Red("❌ Failed to list ingresses: %v", err)
		return nil, "", "", 0, false
	}

	for i := range ingresses.Items {
		ing := &ingresses.Items[i]
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				ingressPath := path.Path
				if ingressPath == "" {
					ingressPath = "/"
				}

				if !strings.HasPrefix(targetPath, ingressPath) {
					continue
				}

				if path.Backend.Service != nil {
					svcName := path.Backend.Service.Name
					var svcPort int32
					if path.Backend.Service.Port.Number != 0 {
						svcPort = path.Backend.Service.Port.Number
					}
					return ing, ingressPath, svcName, svcPort, true
				}
			}
		}
	}

	return nil, "", "", 0, false
}

func printNotFoundTree(namespace, targetPath string) {
	color.Red("🔴 Ingress Not Found\n")
	fmt.Printf("├── Namespace: %s\n", namespace)
	fmt.Printf("├── Target Path: %s\n", targetPath)
	fmt.Printf("└── Result: No ingress rule matches the requested path.\n\n")
}

func printIngressTree(ing *networkingv1.Ingress, path, svcName string, svcPort int32) {
	color.Green("🟢 Ingress Matched\n")
	fmt.Printf("├── Name: %s\n", ing.Name)
	fmt.Printf("├── Namespace: %s\n", ing.Namespace)
	fmt.Printf("├── Matched Path: %s\n", path)
	fmt.Printf("├── Backend Service: %s\n", svcName)
	if svcPort > 0 {
		fmt.Printf("└── Backend Port: %d\n", svcPort)
	} else {
		fmt.Printf("└── Backend Port: (named port)\n")
	}
	fmt.Println()
}

func printServiceErrorTree(svcName string, err error) {
	color.Red("🔴 Service Error (502)\n")
	fmt.Printf("├── Service Name: %s\n", svcName)
	fmt.Printf("└── Error: %v\n\n", err)
}

func printServiceTree(svc *corev1.Service) {
	color.Green("🟢 Service Found\n")
	fmt.Printf("├── Name: %s\n", svc.Name)
	fmt.Printf("├── Type: %s\n", svc.Spec.Type)
	if len(svc.Spec.Selector) > 0 {
		fmt.Printf("└── Selectors:\n")
		for k, v := range svc.Spec.Selector {
			fmt.Printf("    ├── %s: %s\n", k, v)
		}
	} else {
		fmt.Printf("└── Selectors: (none)\n")
	}
	fmt.Println()
}

func filterPodsBySelector(pods []corev1.Pod, selector map[string]string) []corev1.Pod {
	var matched []corev1.Pod
	for i := range pods {
		pod := &pods[i]
		if podMatchesSelector(pod, selector) {
			matched = append(matched, *pod)
		}
	}
	return matched
}

func podMatchesSelector(pod *corev1.Pod, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if pod.Labels[k] != v {
			return false
		}
	}
	return true
}

func isPodReady(pod corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func printPodTree(pods []corev1.Pod, selectors map[string]string) {
	if len(pods) == 0 {
		color.Red("❌ No Matching Pods\n")
		fmt.Printf("└── Result: 0 pods match the service selectors.\n\n")

		color.Yellow("💡 Diagnostic Suggestion\n")
		fmt.Println("   The Service selectors do not match any Pod labels.")
		fmt.Println("   Common causes:")
		fmt.Println("   • Deployment / Pod labels contain a typo or mismatch")
		fmt.Println("   • The workload is deployed in a different namespace")
		fmt.Println("   • The Deployment's label selector differs from the Pod template labels")
		fmt.Println("   • The Service selector key or value is misspelled")
		fmt.Println()

		color.Yellow("   Service Selectors:\n")
		for k, v := range selectors {
			fmt.Printf("      %s: %s\n", k, v)
		}
		fmt.Println()
		return
	}

	color.Green("🟢 Pods Found (%d matching)\n", len(pods))

	allReady := true
	for _, pod := range pods {
		ready := isPodReady(pod)
		if ready {
			color.Green("├── 🟢 %s (Ready)", pod.Name)
			fmt.Println()
		} else {
			color.Red("├── 🔴 %s (Not Ready)", pod.Name)
			fmt.Println()
			allReady = false
		}
	}

	if allReady {
		fmt.Printf("└── ✅ All %d pods are passing readiness probes.\n\n", len(pods))
	} else {
		fmt.Printf("└── ❌ Some pods are failing Kubernetes Readiness Probes.\n\n")
		color.Yellow("💡 Diagnostic Suggestion\n")
		fmt.Println("   Matching pods exist but are not Ready.")
		fmt.Println("   This usually means the application is failing its Readiness Probes.")
		fmt.Println("   • Check `kubectl describe pod <name>` for probe failures")
		fmt.Println("   • Verify the application is listening on the correct port")
		fmt.Println("   • Ensure the readiness probe path/endpoint is implemented and healthy")
		fmt.Println()
	}
}
