/*
Copyright © 2018 inwinSTACK.inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"fmt"
	"reflect"

	"github.com/golang/glog"
	clientset "github.com/inwinstack/blended/client/clientset/versioned"
	opkit "github.com/inwinstack/operator-kit"
	"github.com/inwinstack/pa-svc-syncker/pkg/config"
	"github.com/inwinstack/pa-svc-syncker/pkg/constants"
	"github.com/inwinstack/pa-svc-syncker/pkg/k8sutil"
	"github.com/inwinstack/pa-svc-syncker/pkg/util"
	slice "github.com/thoas/go-funk"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/cache"
)

var Resource = opkit.CustomResource{
	Name:    "service",
	Plural:  "services",
	Version: "v1",
	Kind:    reflect.TypeOf(v1.Service{}).Name(),
}

type ServiceController struct {
	ctx    *opkit.Context
	client clientset.Interface
	conf   *config.OperatorConfig
}

func NewController(ctx *opkit.Context, client clientset.Interface, conf *config.OperatorConfig) *ServiceController {
	return &ServiceController{ctx: ctx, client: client, conf: conf}
}

func (c *ServiceController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	glog.Info("Start watching service resources.")
	watcher := opkit.NewWatcher(Resource, namespace, resourceHandlerFuncs, c.ctx.Clientset.CoreV1().RESTClient())
	go watcher.Watch(&v1.Service{}, stopCh)
	return nil
}

func (c *ServiceController) onAdd(obj interface{}) {
	svc := obj.(*v1.Service).DeepCopy()
	glog.V(2).Infof("Received add on Service %s in %s namespace.", svc.Name, svc.Namespace)

	c.makeAnnotations(svc)
	if err := c.syncSpec(nil, svc); err != nil {
		glog.Errorf("Failed to sync spec on Service %s in %s namespace: %+v.", svc.Name, svc.Namespace, err)
	}
}

func (c *ServiceController) onUpdate(oldObj, newObj interface{}) {
	old := oldObj.(*v1.Service).DeepCopy()
	svc := newObj.(*v1.Service).DeepCopy()
	glog.V(2).Infof("Received update on Service %s in %s namespace.", svc.Name, svc.Namespace)

	if svc.DeletionTimestamp == nil {
		if err := c.syncSpec(old, svc); err != nil {
			glog.Errorf("Failed to sync spec on Service %s in %s namespace: %+v.", svc.Name, svc.Namespace, err)
		}
	}
}

func (c *ServiceController) onDelete(obj interface{}) {
	svc := obj.(*v1.Service).DeepCopy()
	glog.V(2).Infof("Received delete on Service %s in %s namespace.", svc.Name, svc.Namespace)

	if slice.Contains(c.conf.IgnoreNamespaces, svc.Namespace) {
		return
	}

	if len(svc.Spec.Ports) == 0 || len(svc.Spec.ExternalIPs) == 0 {
		return
	}

	if err := c.cleanup(svc); err != nil {
		glog.Errorf("Failed to cleanup on Service %s in %s namespace: %+v.", svc.Name, svc.Namespace, err)
	}
}

func (c *ServiceController) makeAnnotations(svc *v1.Service) {
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}

	if _, ok := svc.Annotations[constants.AnnKeyExternalPool]; !ok {
		svc.Annotations[constants.AnnKeyExternalPool] = constants.DefaultInternetPool
	}
}

func (c *ServiceController) makeRefresh(svc *v1.Service) {
	ip := svc.Annotations[constants.AnnKeyPublicIP]
	if util.ParseIP(ip) == nil {
		svc.Annotations[constants.AnnKeyServiceRefresh] = string(uuid.NewUUID())
	}
}

func (c *ServiceController) syncSpec(old *v1.Service, svc *v1.Service) error {
	if slice.Contains(c.conf.IgnoreNamespaces, svc.Namespace) {
		return nil
	}

	if len(svc.Spec.Ports) == 0 || len(svc.Spec.ExternalIPs) == 0 {
		return nil
	}

	if err := c.allocate(svc); err != nil {
		glog.Errorf("Failed to allocate Public IP: %s.", err)
	}

	addr := svc.Annotations[constants.AnnKeyPublicIP]
	if util.ParseIP(addr) != nil {
		c.syncNAT(svc, addr)
		c.syncSecurity(svc, addr)
	}

	c.makeRefresh(svc)
	if _, err := k8sutil.UpdateService(c.ctx.Clientset, svc.Namespace, svc); err != nil {
		return err
	}
	return nil
}

func (c *ServiceController) allocate(svc *v1.Service) error {
	pool := svc.Annotations[constants.AnnKeyExternalPool]
	public := util.ParseIP(svc.Annotations[constants.AnnKeyPublicIP])
	if public == nil && pool != "" {
		name := svc.Spec.ExternalIPs[0]
		namespace := svc.Namespace
		ip, err := k8sutil.GetIP(c.client, name, namespace)
		if err == nil {
			if ip.Status.Address != "" {
				delete(svc.Annotations, constants.AnnKeyServiceRefresh)
				svc.Annotations[constants.AnnKeyPublicIP] = ip.Status.Address
			}
			return nil
		}

		if _, err := k8sutil.CreateIP(c.client, name, namespace, pool); err != nil {
			return err
		}
	}
	return nil
}

// Sync the PA NAT policies
func (c *ServiceController) syncNAT(svc *v1.Service, addr string) {
	name := fmt.Sprintf("k8s-%s", addr)
	if err := k8sutil.CreateNAT(c.client, name, addr, svc); err != nil {
		glog.Warningf("Failed to create NAT resource: %+v.", err)
	}
}

// Sync the PA Security policies
func (c *ServiceController) syncSecurity(svc *v1.Service, addr string) {
	name := fmt.Sprintf("k8s-%s", addr)

	secPara := &k8sutil.SecurityParameter{
		Name:             name,
		Address:          addr,
		Log:              c.conf.LogSettingName,
		Group:            c.conf.GroupName,
		Services:         c.conf.Services,
		DestinationZones: c.conf.DestinationZones,
	}
	if err := k8sutil.CreateSecurity(c.client, secPara, svc); err != nil {
		glog.Warningf("Failed to create and update Security resource: %+v.", err)
	}
}

func (c *ServiceController) cleanup(svc *v1.Service) error {
	pool := svc.Annotations[constants.AnnKeyExternalPool]
	public := util.ParseIP(svc.Annotations[constants.AnnKeyPublicIP])
	if public != nil && pool != "" {
		namespace := svc.Namespace
		svcs, err := k8sutil.GetServiceList(c.ctx.Clientset, namespace)
		if err != nil {
			return err
		}

		k8sutil.FilterServices(svcs, public.String())
		if len(svcs.Items) != 0 {
			return nil
		}

		if err := k8sutil.DeleteIP(c.client, svc.Spec.ExternalIPs[0], namespace); err != nil {
			return err
		}

		name := fmt.Sprintf("k8s-%s", public.String())
		if err := k8sutil.DeleteSecurity(c.client, name, namespace); err != nil {
			glog.Warningf("Failed to delete Security resource: %+v.", err)
		}

		if err := k8sutil.DeleteNAT(c.client, name, namespace); err != nil {
			glog.Warningf("Failed to delete NAT resource: %+v.", err)
		}
		return nil
	}
	return nil
}
