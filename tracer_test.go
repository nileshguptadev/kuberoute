package main

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTraceRoute_IngressNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	err := TraceRoute(clientset, "default", "/missing")
	if err != nil {
		t.Fatalf("expected no error for missing ingress, got: %v", err)
	}
}

func TestTraceRoute_FullTrace(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
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
			Selector: map[string]string{
				"app": "test-app",
			},
			Ports: []corev1.ServicePort{
				{Port: 8080},
			},
		},
	}

	readyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: ns,
			Labels: map[string]string{
				"app": "test-app",
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	notReadyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-2",
			Namespace: ns,
			Labels: map[string]string{
				"app": "test-app",
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}

	clientset := fake.NewSimpleClientset(ing, svc, readyPod, notReadyPod)

	// Warm up the fake informers or caches if needed — fake client is synchronous
	_, err := clientset.NetworkingV1().Ingresses(ns).Get(ctx, "test-ingress", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	err = TraceRoute(clientset, ns, "/api/users")
	if err != nil {
		t.Fatalf("TraceRoute failed: %v", err)
	}
}

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

func pathTypePtr(pt networkingv1.PathType) *networkingv1.PathType {
	return &pt
}
