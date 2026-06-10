package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RouteResult holds the output of a route trace, separating data collection from rendering.
type RouteResult struct {
	Namespace      string
	TargetPath     string
	IngressFound   bool
	Ingress        *networkingv1.Ingress
	MatchedPath    string
	BackendService string
	BackendPort    int32
	ServiceFound   bool
	Service        *corev1.Service
	ServiceError   error
	PodsFound      bool
	MatchedPods    []corev1.Pod
	Selectors      map[string]string
}

// TraceRouteResult traces the route and returns a RouteResult instead of printing.
// It returns nil, error only on hard API failures; business-logic findings are
// returned as a valid RouteResult with appropriate flags set to false.
func TraceRouteResult(clientset kubernetes.Interface, namespace, targetPath string) (*RouteResult, error) {
	ctx := context.Background()
	result := &RouteResult{
		Namespace:  namespace,
		TargetPath: targetPath,
	}

	// ── Step 1: Ingress Evaluation ────────────────────────────────────────
	matchedIngress, matchedPath, matchedServiceName, matchedServicePort, found := findMatchingIngress(ctx, clientset, namespace, targetPath)
	result.IngressFound = found
	if found {
		result.Ingress = matchedIngress
		result.MatchedPath = matchedPath
		result.BackendService = matchedServiceName
		result.BackendPort = matchedServicePort
	}

	if !found {
		return result, nil
	}

	// ── Step 2: Service Evaluation ────────────────────────────────────────
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, matchedServiceName, metav1.GetOptions{})
	if err != nil {
		result.ServiceError = err
		return result, nil
	}
	result.ServiceFound = true
	result.Service = service

	// ── Step 3: Pod Endpoint Audit ────────────────────────────────────────
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	matchedPods := filterPodsBySelector(pods.Items, service.Spec.Selector)
	result.PodsFound = len(matchedPods) > 0
	result.MatchedPods = matchedPods
	result.Selectors = service.Spec.Selector

	return result, nil
}

// TraceRoute traces the Kubernetes routing path from Ingress -> Service -> Pod for a given namespace and targetPath.
func TraceRoute(clientset kubernetes.Interface, namespace string, targetPath string) error {
	result, err := TraceRouteResult(clientset, namespace, targetPath)
	if err != nil {
		return err
	}
	return RenderRoute(result, os.Stdout)
}

// RenderRoute renders a RouteResult to the provided io.Writer.
// It produces the same tree output as the original print functions.
type routeResultJSON struct {
	Namespace      string            `json:"namespace"`
	TargetPath     string            `json:"targetPath"`
	IngressFound   bool              `json:"ingressFound"`
	IngressName    string            `json:"ingressName,omitempty"`
	MatchedPath    string            `json:"matchedPath,omitempty"`
	BackendService string            `json:"backendService,omitempty"`
	BackendPort    int32             `json:"backendPort,omitempty"`
	ServiceFound   bool              `json:"serviceFound"`
	ServiceName    string            `json:"serviceName,omitempty"`
	ServiceType    string            `json:"serviceType,omitempty"`
	ServiceError   string            `json:"serviceError,omitempty"`
	PodsFound      bool              `json:"podsFound"`
	PodCount       int               `json:"podCount"`
	Pods           []podJSON         `json:"pods,omitempty"`
	Selectors      map[string]string `json:"selectors,omitempty"`
}

type podJSON struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Status string `json:"status"`
}

// RenderRouteJSON writes a clean JSON representation of a RouteResult to w.
func RenderRouteJSON(result *RouteResult, w io.Writer) error {
	rj := routeResultJSON{
		Namespace:    result.Namespace,
		TargetPath:   result.TargetPath,
		IngressFound: result.IngressFound,
	}

	if result.IngressFound && result.Ingress != nil {
		rj.IngressName = result.Ingress.Name
		rj.MatchedPath = result.MatchedPath
		rj.BackendService = result.BackendService
		rj.BackendPort = result.BackendPort
	}

	rj.ServiceFound = result.ServiceFound
	if result.ServiceFound && result.Service != nil {
		rj.ServiceName = result.Service.Name
		rj.ServiceType = string(result.Service.Spec.Type)
	} else if result.ServiceError != nil {
		rj.ServiceError = result.ServiceError.Error()
	}

	rj.PodsFound = result.PodsFound
	rj.PodCount = len(result.MatchedPods)
	for _, pod := range result.MatchedPods {
		rj.Pods = append(rj.Pods, podJSON{
			Name:   pod.Name,
			Ready:  isPodReady(pod),
			Status: string(pod.Status.Phase),
		})
	}
	rj.Selectors = result.Selectors

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rj)
}

func RenderRoute(result *RouteResult, w io.Writer) error {
	// Step 1: Ingress not found
	if !result.IngressFound {
		renderNotFoundTree(result, w)
		return nil
	}

	// Step 1: Ingress matched
	renderIngressTree(result, w)

	// Step 2: Service error
	if result.ServiceError != nil {
		renderServiceErrorTree(result, w)
		return nil
	}

	// Step 2: Service found
	renderServiceTree(result, w)

	// Step 3: Pods
	renderPodTree(result, w)

	return nil
}

func renderNotFoundTree(result *RouteResult, w io.Writer) {
	color.Red("🔴 Ingress Not Found\n")
	fmt.Fprintf(w, "├── Namespace: %s\n", result.Namespace)
	fmt.Fprintf(w, "├── Target Path: %s\n", result.TargetPath)
	fmt.Fprintf(w, "└── Result: No ingress rule matches the requested path.\n\n")
}

func renderIngressTree(result *RouteResult, w io.Writer) {
	color.Green("🟢 Ingress Matched\n")
	fmt.Fprintf(w, "├── Name: %s\n", result.Ingress.Name)
	fmt.Fprintf(w, "├── Namespace: %s\n", result.Ingress.Namespace)
	fmt.Fprintf(w, "├── Matched Path: %s\n", result.MatchedPath)
	fmt.Fprintf(w, "├── Backend Service: %s\n", result.BackendService)
	if result.BackendPort > 0 {
		fmt.Fprintf(w, "└── Backend Port: %d\n", result.BackendPort)
	} else {
		fmt.Fprintf(w, "└── Backend Port: (named port)\n")
	}
	fmt.Fprintln(w)
}

func renderServiceErrorTree(result *RouteResult, w io.Writer) {
	color.Red("🔴 Service Error (502)\n")
	fmt.Fprintf(w, "├── Service Name: %s\n", result.BackendService)
	fmt.Fprintf(w, "└── Error: %v\n\n", result.ServiceError)
}

func renderServiceTree(result *RouteResult, w io.Writer) {
	color.Green("🟢 Service Found\n")
	fmt.Fprintf(w, "├── Name: %s\n", result.Service.Name)
	fmt.Fprintf(w, "├── Type: %s\n", result.Service.Spec.Type)
	if len(result.Service.Spec.Selector) > 0 {
		fmt.Fprintf(w, "└── Selectors:\n")
		for k, v := range result.Service.Spec.Selector {
			fmt.Fprintf(w, "    ├── %s: %s\n", k, v)
		}
	} else {
		fmt.Fprintf(w, "└── Selectors: (none)\n")
	}
	fmt.Fprintln(w)
}

func renderPodTree(result *RouteResult, w io.Writer) {
	if len(result.MatchedPods) == 0 {
		color.Red("❌ No Matching Pods\n")
		fmt.Fprintf(w, "└── Result: 0 pods match the service selectors.\n\n")

		color.Yellow("💡 Diagnostic Suggestion\n")
		fmt.Fprintln(w, "   The Service selectors do not match any Pod labels.")
		fmt.Fprintln(w, "   Common causes:")
		fmt.Fprintln(w, "   • Deployment / Pod labels contain a typo or mismatch")
		fmt.Fprintln(w, "   • The workload is deployed in a different namespace")
		fmt.Fprintln(w, "   • The Deployment's label selector differs from the Pod template labels")
		fmt.Fprintln(w, "   • The Service selector key or value is misspelled")
		fmt.Fprintln(w)

		color.Yellow("   Service Selectors:\n")
		for k, v := range result.Selectors {
			fmt.Fprintf(w, "      %s: %s\n", k, v)
		}
		fmt.Fprintln(w)
		return
	}

	color.Green("🟢 Pods Found (%d matching)\n", len(result.MatchedPods))

	allReady := true
	for _, pod := range result.MatchedPods {
		ready := isPodReady(pod)
		if ready {
			color.Green("├── 🟢 %s (Ready)", pod.Name)
			fmt.Fprintln(w)
		} else {
			color.Red("├── 🔴 %s (Not Ready)", pod.Name)
			fmt.Fprintln(w)
			allReady = false
		}
	}

	if allReady {
		fmt.Fprintf(w, "└── ✅ All %d pods are passing readiness probes.\n\n", len(result.MatchedPods))
	} else {
		fmt.Fprintf(w, "└── ❌ Some pods are failing Kubernetes Readiness Probes.\n\n")
		color.Yellow("💡 Diagnostic Suggestion\n")
		fmt.Fprintln(w, "   Matching pods exist but are not Ready.")
		fmt.Fprintln(w, "   This usually means the application is failing its Readiness Probes.")
		fmt.Fprintln(w, "   • Check `kubectl describe pod <name>` for probe failures")
		fmt.Fprintln(w, "   • Verify the application is listening on the correct port")
		fmt.Fprintln(w, "   • Ensure the readiness probe path/endpoint is implemented and healthy")
		fmt.Fprintln(w)
	}
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
