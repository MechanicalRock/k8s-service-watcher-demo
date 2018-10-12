package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"os"
	"os/signal"
	"syscall"

	// "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/api/core/v1"

	kubeinformers "k8s.io/client-go/informers"

	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// depending on how you are configured, you may want to add
	// other auth providers
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var kubeconfig string

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "~/.kube/config", "Location of kubeconfig")
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if config, err := clientcmd.BuildConfigFromFlags("", kubeconfig); err == nil {
		return config, err
	}

	// else try running with the from the default kubeconfig location
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}

func main() {

	signalChannel := make(chan os.Signal, 2)

	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	config, err := buildConfig(kubeconfig)
	if err != nil {
		panic(err)
	}

	// The resync period, if specified x > 0, will send
	// the entire list of watched resources to the client
	// every x * period.

	// Clients will typically need to set this if they are
	// behaving as a controller - if the controller dies
	// it will need to get back up to "resync" by
	// interrogating the current state
	resyncPeriod := 5 * time.Second

	namespace := "kube-system"

	// I'll admit the API for filtering out resources
	// is a tad.... baroque.

	/*listOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = labels.Set(map[string]string{
			"test": "test",
		}).AsSelector().String()
	}*/

	// We need to make a handler that will receive the events
	// from the informer. For the sake of the example I'll
	// just throw them onto the queue.
	events := make(chan interface{})
	fn := func(obj interface{}) {
		events <- obj
	}

	handler := &cache.ResourceEventHandlerFuncs{
		AddFunc:    fn,
		DeleteFunc: fn,
		UpdateFunc: func(old interface{}, new interface{}) {
			fn(new)
		},
	}

	// NewForConfigOrDie panics if the configuration throws an error
	kubeclientset := kubernetes.NewForConfigOrDie(config)
	kubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(
		kubeclientset, resyncPeriod, namespace, nil)

	serviceInformer := kubeInformerFactory.Core().V1().Services()
	// serviceLister := serviceInformer.Lister()
	serviceInformer.Informer().AddEventHandler(handler)

	ctx, cancel := context.WithCancel(context.Background())

	go kubeInformerFactory.Start(ctx.Done())

	// Generally it is a good idea to wait for the informer
	// cache to sync. client-go provides a helper to
	// do this...
	if !cache.WaitForCacheSync(ctx.Done(),
		serviceInformer.Informer().HasSynced) {
		os.Exit(1)
	}

	// At this point we can receive and the events and do
	// whatever we like with them. If we receive SIGTERM
	// we cancel the context and exit.

	for {
		select {
		case event := <-events:
			service, ok := event.(*v1.Service)
			if ok {
				fmt.Printf("%s\t\t%s\t\t%s\n", service.Namespace, service.Name, service.Spec.Type)
			}
		case <-signalChannel:
			cancel()
			os.Exit(0)
		}
	}
}
