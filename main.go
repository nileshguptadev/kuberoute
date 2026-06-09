package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var (
		namespace string
		targetPath string
	)

	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to trace")
	flag.StringVar(&targetPath, "path", "/", "HTTP path to trace through ingress rules")
	flag.Parse()

	if err := run(namespace, targetPath); err != nil {
		color.Red("❌ KubeRoute failed: %v\n", err)
		os.Exit(1)
	}
}

func run(namespace, targetPath string) error {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("unable to load kubeconfig from %s: %w", kubeconfigPath, err)
	}

	config.Burst = 10
	config.QPS = 5

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("unable to create Kubernetes clientset: %w", err)
	}

	color.Cyan("🔍 KubeRoute — Tracing path %q in namespace %q\n\n", targetPath, namespace)
	return TraceRoute(clientset, namespace, targetPath)
}
