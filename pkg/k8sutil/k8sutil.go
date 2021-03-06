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

package k8sutil

import (
	"github.com/inwinstack/pa-svc-syncker/pkg/constants"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("master", kubeconfig)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func GetServiceList(c kubernetes.Interface, namespace string) (*v1.ServiceList, error) {
	return c.CoreV1().Services(namespace).List(metav1.ListOptions{})
}

func UpdateService(c kubernetes.Interface, namespace string, svc *v1.Service) (*v1.Service, error) {
	return c.CoreV1().Services(namespace).Update(svc)
}

func FilterServices(svcs *v1.ServiceList, addr string) {
	var items []v1.Service
	for _, svc := range svcs.Items {
		v := svc.Annotations[constants.AnnKeyPublicIP]
		if v == addr {
			items = append(items, svc)
		}
	}
	svcs.Items = items
}
