package client

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Client struct {
	*kubernetes.Clientset
}

func getClientSet() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset := kubernetes.NewForConfigOrDie(config)

	return clientset, nil
}

func (client *Client) getNodes() error {

	nodes, err := client.CoreV1().Nodes().List(v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, _ = range nodes.Items {
		//
	}

	return nil
}
