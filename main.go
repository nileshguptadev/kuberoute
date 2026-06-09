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
		namespace     string
		targetPath    string
		kubeconfigFlag string
	)

	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to trace")
	flag.StringVar(&targetPath, "path", "/", "HTTP path to trace through ingress rules")
	flag.StringVar(&kubeconfigFlag, "kubeconfig", "", "Path to kubeconfig file")
	flag.Parse()

	// Resolve kubeconfig path with kubectl-compatible fallback chain
	kubeconfig := kubeconfigFlag
	if kubeconfig == "" {
		if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
			kubeconfig = envKubeconfig
		} else {
			home := homedir.HomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	if err := run(namespace, targetPath, kubeconfig); err != nil {
		color.Red("❌ KubeRoute failed: %v\n", err)
		os.Exit(1)
	}
}

func run(namespace, targetPath, kubeconfig string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("unable to load kubeconfig from %s: %w", kubeconfig, err)
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
