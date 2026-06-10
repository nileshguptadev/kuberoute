package main

import (
	"bytes"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestTraceRouteResult_IngressNotFound verifies that when no ingress exists,
// RouteResult reflects IngressFound=false and all downstream flags are false.
func TestTraceRouteResult_IngressNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	result, err := TraceRouteResult(clientset, "test-ns", "/missing")
	if err != nil {
		t.Fatalf("TraceRouteResult returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}
	if result.IngressFound {
		t.Error("expected IngressFound=false, got true")
	}
	if result.ServiceFound {
		t.Error("expected ServiceFound=false, got true")
	}
	if result.ServiceError != nil {
		t.Errorf("expected ServiceError=nil, got %v", result.ServiceError)
	}
	if result.PodsFound {
		t.Error("expected PodsFound=false, got true")
	}
}

// TestTraceRouteResult_FullHealthy verifies the full happy path:
// ingress -> service -> 2 ready pods, all fields populated correctly.
func TestTraceRouteResult_FullHealthy(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "test-svc",
											Port: networkingv1.ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test-app"},
			Ports:    []corev1.ServicePort{{Port: 8080}},
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: ns,
			Labels:    map[string]string{"app": "test-app"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: ns,
			Labels:    map[string]string{"app": "test-app"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc, pod1, pod2)

	result, err := TraceRouteResult(clientset, ns, "/api/users")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if result.BackendService != "test-svc" {
		t.Errorf("expected BackendService=test-svc, got %s", result.BackendService)
	}
	if result.BackendPort != 8080 {
		t.Errorf("expected BackendPort=8080, got %d", result.BackendPort)
	}
	if !result.ServiceFound {
		t.Error("expected ServiceFound=true")
	}
	if result.ServiceError != nil {
		t.Errorf("expected ServiceError=nil, got %v", result.ServiceError)
	}
	if !result.PodsFound {
		t.Error("expected PodsFound=true")
	}
	if len(result.MatchedPods) != 2 {
		t.Fatalf("expected 2 matched pods, got %d", len(result.MatchedPods))
	}
	for _, pod := range result.MatchedPods {
		if !isPodReady(pod) {
			t.Errorf("pod %s should be ready", pod.Name)
		}
	}
}

// TestTraceRouteResult_ServiceNotFound verifies that a missing service
// sets ServiceFound=false and ServiceError != nil.
func TestTraceRouteResult_ServiceNotFound(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-svc-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "missing-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// No service object — fake clientset is empty
	clientset := fake.NewSimpleClientset(ing)

	result, err := TraceRouteResult(clientset, ns, "/api")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if result.BackendService != "missing-svc" {
		t.Errorf("expected BackendService=missing-svc, got %s", result.BackendService)
	}
	if result.ServiceFound {
		t.Error("expected ServiceFound=false, got true")
	}
	if result.ServiceError == nil {
		t.Fatal("expected ServiceError != nil, got nil")
	}
	if result.PodsFound {
		t.Error("expected PodsFound=false, got true")
	}
}

// TestTraceRouteResult_NoMatchingPods verifies that when no pods match the
// service selector, PodsFound=false and len(MatchedPods)=0.
func TestTraceRouteResult_NoMatchingPods(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-pods-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "foo-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "foo"},
			Ports:    []corev1.ServicePort{{Port: 80}},
		},
	}

	// No pods in the fake clientset
	clientset := fake.NewSimpleClientset(ing, svc)

	result, err := TraceRouteResult(clientset, ns, "/")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if !result.ServiceFound {
		t.Error("expected ServiceFound=true")
	}
	if result.PodsFound {
		t.Error("expected PodsFound=false, got true")
	}
	if len(result.MatchedPods) != 0 {
		t.Fatalf("expected 0 matched pods, got %d", len(result.MatchedPods))
	}
	if result.Selectors == nil {
		t.Fatal("expected Selectors != nil")
	}
	if result.Selectors["app"] != "foo" {
		t.Errorf("expected Selectors[app]=foo, got %s", result.Selectors["app"])
	}
}

// TestTraceRouteResult_PodsNotReady verifies that a pod with PodReady=ConditionFalse
// is still included in MatchedPods but isPodReady returns false.
func TestTraceRouteResult_PodsNotReady(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-ready-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "api"},
			Ports:    []corev1.ServicePort{{Port: 80}},
		},
	}

	notReadyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod-1",
			Namespace: ns,
			Labels:    map[string]string{"app": "api"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc, notReadyPod)

	result, err := TraceRouteResult(clientset, ns, "/")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if !result.ServiceFound {
		t.Error("expected ServiceFound=true")
	}
	if !result.PodsFound {
		t.Error("expected PodsFound=true (pods exist but are not ready)")
	}
	if len(result.MatchedPods) != 1 {
		t.Fatalf("expected 1 matched pod, got %d", len(result.MatchedPods))
	}
	if isPodReady(result.MatchedPods[0]) {
		t.Error("expected isPodReady=false for the not-ready pod")
	}
}

// TestTraceRouteResult_LabelMismatch verifies that a pod with a mismatching
// label is not included in MatchedPods.
func TestTraceRouteResult_LabelMismatch(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mismatch-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "mismatch-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mismatch-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "v2"},
			Ports:    []corev1.ServicePort{{Port: 80}},
		},
	}

	// Pod has app=v1 but service selector is app=v2
	wrongPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "v1-pod",
			Namespace: ns,
			Labels:    map[string]string{"app": "v1"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc, wrongPod)

	result, err := TraceRouteResult(clientset, ns, "/")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if !result.ServiceFound {
		t.Error("expected ServiceFound=true")
	}
	if result.PodsFound {
		t.Error("expected PodsFound=false (no pods match selector), got true")
	}
	if len(result.MatchedPods) != 0 {
		t.Fatalf("expected 0 matched pods, got %d", len(result.MatchedPods))
	}
}

// TestTraceRouteResult_NamedPort verifies that an ingress using a named port
// (Number=0) sets BackendPort=0 to indicate a named port.
func TestTraceRouteResult_NamedPort(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "named-port-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "named-svc",
											Port: networkingv1.ServiceBackendPort{Name: "http"}, // Number=0
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "named-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 8080}},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc)

	result, err := TraceRouteResult(clientset, ns, "/")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if result.BackendService != "named-svc" {
		t.Errorf("expected BackendService=named-svc, got %s", result.BackendService)
	}
	if result.BackendPort != 0 {
		t.Errorf("expected BackendPort=0 (named port), got %d", result.BackendPort)
	}
}

// TestTraceRouteResult_MultipleIngressRules verifies that when multiple
// ingresses exist, the one with the matching path is selected.
func TestTraceRouteResult_MultipleIngressRules(t *testing.T) {
	ns := "test-ns"

	ing1 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/other",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "other-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ing2 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-svc",
											Port: networkingv1.ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing1, ing2)

	result, err := TraceRouteResult(clientset, ns, "/api/users")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("TraceRouteResult returned nil result")
	}

	if !result.IngressFound {
		t.Error("expected IngressFound=true")
	}
	if result.MatchedPath != "/api" {
		t.Errorf("expected MatchedPath=/api, got %s", result.MatchedPath)
	}
	if result.BackendService != "api-svc" {
		t.Errorf("expected BackendService=api-svc, got %s", result.BackendService)
	}
}

// TestFilterPodsBySelector verifies that filterPodsBySelector correctly
// filters pods by label selector.
func TestFilterPodsBySelector(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "web", "tier": "frontend"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "api", "tier": "backend"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "web"},
			},
		},
	}

	selector := map[string]string{"app": "web"}
	matched := filterPodsBySelector(pods, selector)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matched pods, got %d", len(matched))
	}
}

// TestPodMatchesSelector is a table-driven test for podMatchesSelector.
func TestPodMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		selector map[string]string
		want     bool
	}{
		{
			name:     "exact match",
			labels:   map[string]string{"app": "web"},
			selector: map[string]string{"app": "web"},
			want:     true,
		},
		{
			name:     "missing label",
			labels:   map[string]string{},
			selector: map[string]string{"app": "web"},
			want:     false,
		},
		{
			name:     "mismatch value",
			labels:   map[string]string{"app": "api"},
			selector: map[string]string{"app": "web"},
			want:     false,
		},
		{
			name:     "extra labels ok",
			labels:   map[string]string{"app": "web", "tier": "frontend"},
			selector: map[string]string{"app": "web"},
			want:     true,
		},
		{
			name:     "empty selector",
			labels:   map[string]string{"app": "web"},
			selector: map[string]string{},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: tt.labels},
			}
			got := podMatchesSelector(pod, tt.selector)
			if got != tt.want {
				t.Fatalf("podMatchesSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderRouteJSON(t *testing.T) {
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ing",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-svc",
											Port: networkingv1.ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-svc",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "api"},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod-1",
			Namespace: ns,
			Labels:    map[string]string{"app": "api"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod-2",
			Namespace: ns,
			Labels:    map[string]string{"app": "api"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc, pod1, pod2)

	result, err := TraceRouteResult(clientset, ns, "/api/users")
	if err != nil {
		t.Fatalf("TraceRouteResult returned error: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderRouteJSON(result, &buf); err != nil {
		t.Fatalf("RenderRouteJSON returned error: %v", err)
	}

	var rj map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &rj); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if rj["namespace"] != ns {
		t.Errorf("expected namespace=%q, got %q", ns, rj["namespace"])
	}
	if rj["targetPath"] != "/api/users" {
		t.Errorf("expected targetPath=%q, got %q", "/api/users", rj["targetPath"])
	}
	if rj["ingressFound"] != true {
		t.Errorf("expected ingressFound=true, got %v", rj["ingressFound"])
	}
	if rj["ingressName"] != "test-ing" {
		t.Errorf("expected ingressName=%q, got %v", "test-ing", rj["ingressName"])
	}
	if rj["matchedPath"] != "/api" {
		t.Errorf("expected matchedPath=%q, got %v", "/api", rj["matchedPath"])
	}
	if rj["backendService"] != "api-svc" {
		t.Errorf("expected backendService=%q, got %v", "api-svc", rj["backendService"])
	}
	if rj["backendPort"] != float64(8080) {
		t.Errorf("expected backendPort=8080, got %v", rj["backendPort"])
	}
	if rj["serviceFound"] != true {
		t.Errorf("expected serviceFound=true, got %v", rj["serviceFound"])
	}
	if rj["serviceName"] != "api-svc" {
		t.Errorf("expected serviceName=%q, got %v", "api-svc", rj["serviceName"])
	}
	if rj["serviceType"] != "ClusterIP" {
		t.Errorf("expected serviceType=%q, got %v", "ClusterIP", rj["serviceType"])
	}
	if rj["podsFound"] != true {
		t.Errorf("expected podsFound=true, got %v", rj["podsFound"])
	}
	if rj["podCount"] != float64(2) {
		t.Errorf("expected podCount=2, got %v", rj["podCount"])
	}
	pods, ok := rj["pods"].([]interface{})
	if !ok || len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %v", rj["pods"])
	}
	for _, p := range pods {
		podMap := p.(map[string]interface{})
		if podMap["ready"] != true {
			t.Errorf("expected pod ready=true, got %v", podMap["ready"])
		}
		if podMap["status"] != "Running" {
			t.Errorf("expected pod status=Running, got %v", podMap["status"])
		}
	}
	selectors, ok := rj["selectors"].(map[string]interface{})
	if !ok || selectors["app"] != "api" {
		t.Errorf("expected selectors[app]=api, got %v", rj["selectors"])
	}
}

func pathTypePtr(pt networkingv1.PathType) *networkingv1.PathType {
	return &pt
}
