package barrel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	clientset "github.com/hexiaodai/virtnet/pkg/k8s/client/clientset/versioned"
	"github.com/hexiaodai/virtnet/pkg/k8s/client/informers/externalversions"
	listers "github.com/hexiaodai/virtnet/pkg/k8s/client/listers/virtnet/v1alpha1"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var informerLogger *zap.SugaredLogger

type BarrelInformer struct {
	client          client.Client
	barrelLister    listers.BarrelLister
	barrelSynced    cache.InformerSynced
	barrelWorkqueue workqueue.RateLimitingInterface

	pools sync.Map
}

func NewBarrelInformer(client client.Client) *BarrelInformer {
	if informerLogger == nil {
		informerLogger = logger.Named("Informer")
	}
	return &BarrelInformer{
		client: client,
		pools:  sync.Map{},
	}
}

func (bi *BarrelInformer) SetupInformer(ctx context.Context, clientInter clientset.Interface) error {
	informerLogger.Debug("Setting up informer with SharedInformerFactory")

	factory := externalversions.NewSharedInformerFactory(clientInter, 0)
	barrelInformer := factory.Virtnest().V1alpha1().Barrels()

	bi.barrelLister = barrelInformer.Lister()
	bi.barrelSynced = barrelInformer.Informer().HasSynced
	bi.barrelWorkqueue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Barrels")

	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			barrel := obj.(*v1alpha1.Barrel)
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				bi.barrelWorkqueue.Add(key)
				informerLogger.Debugf("added '%s' to Barrel workqueue", barrel.Name)
			} else {
				informerLogger.Errorf("failed to add '%s' to Barrel workqueue, error: %v", barrel.Name, err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			// barrel := newObj.(*v1alpha1.Barrel)
			// key, err := cache.MetaNamespaceKeyFunc(newObj)
			// if err == nil {
			// 	bi.barrelWorkqueue.Add(key)
			// 	informerLogger.Debugf("updated '%s' to Barrel workqueue", barrel.Name)
			// } else {
			// 	informerLogger.Errorf("failed to update '%s' to Barrel workqueue, error: %v", barrel.Name, err)
			// }
		},
		DeleteFunc: func(obj interface{}) {
			barrel := obj.(*v1alpha1.Barrel)
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				bi.barrelWorkqueue.Add(key)
				informerLogger.Debugf("deleted '%s' to Barrel workqueue", barrel.Name)
			} else {
				informerLogger.Errorf("failed to delete '%s' to Barrel workqueue, error: %v", barrel.Name, err)
			}
		},
	}
	if _, err := barrelInformer.Informer().AddEventHandler(eventHandler); err != nil {
		return fmt.Errorf("failed to AddEventHandler, error: %v", err)
	}

	factory.Start(ctx.Done())

	go func() {
		if err := bi.run(ctx.Done()); err != nil {
			informerLogger.Fatalf("failed to run barrel controller, error: %v", err)
		}
	}()

	return nil
}

func (bi *BarrelInformer) run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer bi.barrelWorkqueue.ShutDown()
	informerLogger.Debug("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, bi.barrelSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	for i := 0; i < 1; i++ {
		informerLogger.Debugf("Starting Normal Barrel processing worker '%d'", i)
		go wait.Until(bi.runWorker, 1*time.Second, stopCh)
	}

	<-stopCh
	informerLogger.Error("Shutting down Barrel controller workers")
	return nil
}

func (bi *BarrelInformer) runWorker() {
	for bi.processNextWorkItem() {
	}
}

func (bi *BarrelInformer) processNextWorkItem() bool {
	key, quit := bi.barrelWorkqueue.Get()
	if quit {
		return false
	}
	defer bi.barrelWorkqueue.Done(key)

	obj, err := bi.barrelLister.Get(key.(string))
	if err != nil {
		if apierrors.IsNotFound(err) {
			bi.pools.Load()
		}
	}

	return true
}

func (bi *BarrelInformer) handleErr(err error, key interface{}) {
	if err == nil {
		bi.barrelWorkqueue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if bi.barrelWorkqueue.NumRequeues(key) < 5 {
		informerLogger.Debugf("Error syncing barrel %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		bi.barrelWorkqueue.AddRateLimited(key)
		return
	}

	bi.barrelWorkqueue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	informerLogger.Errorf("Dropping barrel %q out of the queue: %v", key, err)
}
