package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset  kubernetes.Interface
	lister     appslisters.DeploymentLister
	cacheSyncd cache.InformerSynced
	queue      workqueue.RateLimitingInterface
	workers    int
}

func New(clientset kubernetes.Interface, depInformer appsinformers.DeploymentInformer, workers int) *controller {
	c := &controller{
		clientset:  clientset,
		lister:     depInformer.Lister(),
		cacheSyncd: depInformer.Informer().HasSynced,
		queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "expose"),
		workers:    workers,
	}
	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			DeleteFunc: c.handleDelete,
		},
	)
	return c
}

func (c *controller) handleAdd(obj interface{}) {
	fmt.Println("Add func called\n")
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err == nil {
		c.queue.Add(key)
	}

}

func (c *controller) handleDelete(obj interface{}) {
	fmt.Println("Delete func called\n")
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err == nil {
		c.queue.Add(key)
	}
}

func (c *controller) Run(ch <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	fmt.Println("Starting the controller\n")

	if !cache.WaitForCacheSync(ch, c.cacheSyncd) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}
	for i := 0; i <= c.workers; i++ {
		go wait.Until(c.runWorker, 1*time.Second, ch)
	}
	<-ch
}
func (c *controller) processNextItem() bool {
	// Wait until there is new item in the queue

	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)
	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))

	if err != nil {
		fmt.Printf("Spliting name & namespace Error: %s\n", err.Error())
		return false
	}

	//Check if object deleted from cluster
	dep, err := c.clientset.AppsV1().Deployments(ns).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Printf("Deployment %s event delete \n", dep.Name)
		err = c.clientset.CoreV1().Services(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("deleting service %s error: %s\n", dep.Name, err.Error())
			return false
		}

		err = c.clientset.NetworkingV1().Ingresses(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("deleting ingress %s error: %s\n", dep.Name, err.Error())
			return false
		}
		return true
	}
	err = c.syncDeployment(ns, name)
	if err != nil {
		c.handleError(key)
		fmt.Printf("Syncing deployment %s\n", err.Error())
		return false
	}
	//If processed properly dropping from queue
	c.queue.Forget(key)
	return true
}

func (c *controller) handleError(key interface{}) {
	if c.queue.NumRequeues(key) < 5 {
		fmt.Printf("Error syncing %v\n", key)
		c.queue.AddRateLimited(key)
	}
}

func (c *controller) syncDeployment(ns, name string) error {
	dep, err := c.lister.Deployments(ns).Get(name)
	if err != nil {
		fmt.Printf("Error getting Deployment: %s\n", err.Error())
		return err
	}

	//Find service if exists already
	//svcs, err := c.clientset.Core().Services(ns).List(metav1.ListOptions{})
	//fmt.Printf("Services %v\n", svcs)
	//find conatiner ports
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: podLabels(*dep),
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
					//TargetPort: intstr.FromInt(80),
				},
			},
		},
	}
	s, err := c.clientset.CoreV1().Services(ns).Create(context.Background(), &svc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Service failed: %s\n", err.Error())
		return err
	}
	return createIngress(context.Background(), c.clientset, s)
}

func createIngress(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	pathType := "Prefix"
	className := "nginx"
	ingress := netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &className,
			Rules: []netv1.IngressRule{
				{
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     fmt.Sprintf("/%s", svc.Name),
									PathType: (*netv1.PathType)(&pathType),
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: svc.Name,
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
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

	_, err := client.NetworkingV1().Ingresses(svc.Namespace).Create(ctx, &ingress, metav1.CreateOptions{})
	return err
}
func (c *controller) runWorker() {
	for c.processNextItem() {

	}
}

func podLabels(dep appsv1.Deployment) map[string]string {
	return dep.Spec.Template.Labels
}
