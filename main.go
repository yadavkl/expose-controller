package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/yadavkl/expose/controller"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "/Users/klyadav/.kube/config", "Location of your config file")
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	if err != nil {
		fmt.Printf("Error while building config from flags %s\n", err.Error())
		config, err = rest.InClusterConfig()
		if err != nil {
			fmt.Printf("Error %s getting inclusterconfig\n", err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		fmt.Printf("Error: %s , creating clientset\n")
	}
	ch := make(chan struct{})
	informers := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	ctrl := controller.New(
		clientset,
		informers.Apps().V1().Deployments(),
		informers.Networking().V1().Ingresses(),
		informers.Core().V1().Services(),
		5,
	)

	informers.Start(ch)
	ctrl.Run(ch)

}
