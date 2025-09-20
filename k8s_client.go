package main

import (
	"fmt"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	istio "istio.io/client-go/pkg/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/client-go/rest"
)

// Client represents a client that can interact with kubernetes resources
type Client struct {
	Kube   *kubernetes.Clientset
	Istio  *istio.Clientset
	Config *rest.Config

	log *logrus.Logger
}

// InferClient returns a new client based on the env
func InferClient() (*Client, error) {
	// If `/root/.kube/config` exists, use it
	_, err := os.Stat("/root/.kube/config")
	if err == nil {
		return NewDevClient("/root/.kube/config")
	}
	// fmt.Printf("No kubeconfig found at /root/.kube/config: %v\n", err)

	if configPath := os.Getenv("KUBECONFIG"); configPath != "" {
		return NewDevClient(configPath)
	}

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		fmt.Println("Running in a pod, using InClusterConfig")
		return NewClientForPod()
	}

	fmt.Println("Running locally, using kubeconfig")
	home := os.Getenv("HOME")
	return NewDevClient(home + "/.kube/config")
}

// NewDevClient returns a k8s client
func NewDevClient(path string) (*Client, error) {
	fmt.Println("Using kubeconfig:", path)

	configOverrides := &clientcmd.ConfigOverrides{}
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: path,
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		log.Fatalf("Failed to build config: %v", err)
	}
	return NewClientFromConfig(config)
}

// NewClientForPod returns a k8s client
// Used when the caller is in a pod in a k8s cluster
// It looks in /var/run/secrets/kubernetes.io/serviceaccount/token
func NewClientForPod() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to build config: %v", err)
	}

	return NewClientFromConfig(config)
}

// NewClientFromConfig returns a new client from a given rest config
func NewClientFromConfig(config *rest.Config) (*Client, error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
	}

	istioClient, err := istio.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Istio clientset: %v", err)
	}

	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	log.SetLevel(logrus.InfoLevel)

	return &Client{kubeClient, istioClient, config, log}, nil
}
