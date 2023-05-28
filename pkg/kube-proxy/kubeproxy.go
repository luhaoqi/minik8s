package kube_proxy

import (
	"encoding/json"
	"fmt"
	"minik8s/cmd/kube-apiserver/app/apiconfig"
	"minik8s/pkg/client/informer"
	"minik8s/pkg/client/tool"
	"minik8s/pkg/service"
	kubeservice "minik8s/pkg/service"
)

func NewKubeProxy() *KubeProxy {
	res := &KubeProxy{}
	res.ServiceInformer = informer.NewInformer(apiconfig.SERVICE_PATH)
	res.ServiceName2SvcChain = make(map[string]map[string]*SvcChain)
	res.DNSManager = NewDnsManager()
	return res
}

func (proxy *KubeProxy) updateRuntimeServiceHandler(event tool.Event) {
	prefix := "[kubeproxy][addService]"
	fmt.Println(prefix + "key:" + event.Key)
	runtimeService := &kubeservice.Service{}
	err := json.Unmarshal([]byte(event.Val), runtimeService)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if len(runtimeService.PodNameAndIps) == 0 {
		fmt.Println(prefix + "waiting for select pods..")
		return
	}
	svcChain, ok := proxy.ServiceName2SvcChain[event.Key]
	if !ok { // 创建runtime-service并修改iptable
		if runtimeService.ServiceSpec.Status.Phase == service.ServiceErrorPhase {
			return
		}
		svcChain = make(map[string]*SvcChain)
		for _, val := range runtimeService.ServiceSpec.Ports {
			var infos []*PodInfo
			for _, pod := range runtimeService.PodNameAndIps {
				infos = append(infos, &PodInfo{
					IP:   pod.Ip,
					Name: pod.Name,
					Port: val.TargetPort,
				})
			}
			cur := NewSvcChain(runtimeService.ServiceMeta.Name, "nat", "SERVICE", runtimeService.ServiceSpec.ClusterIP, val.Port, val.Protocol, infos)
			err1 := cur.ApplyChain()
			if err1 != nil {
				fmt.Println(err1.Error())
			}
			svcChain[cur.Name] = cur
		}
		proxy.ServiceName2SvcChain[event.Key] = svcChain
	} else { // 更新iptable
		for _, val := range runtimeService.ServiceSpec.Ports {
			var infos []*PodInfo
			for _, pod := range runtimeService.PodNameAndIps {
				infos = append(infos, &PodInfo{
					IP:   pod.Ip,
					Name: pod.Name,
					Port: val.TargetPort,
				})
			}
			key := SvcChainPrefix + "-" + runtimeService.ServiceMeta.Name + val.Port // key for svcchain
			target, ok := svcChain[key]
			if !ok {
				fmt.Println("[kubeproxy], not found svcchain key=" + key)
				return
			}
			target.UpdateChain(infos)
		}
	}
}

// key-> serviceName
// val-> runtimeService
func (proxy *KubeProxy) deleteRuntimeServiceHandler(event tool.Event) {
	prefix := "[kubeproxy][deleteService]"
	fmt.Println(prefix + "key:" + event.Key)
	SvcChain, ok := proxy.ServiceName2SvcChain[event.Key]
	if !ok {
		fmt.Println(prefix + "no such service")
		return
	} else {
		for _, v := range SvcChain {
			err := v.DeleteChain()
			if err != nil {
				fmt.Println(err.Error())
				return
			}
		}
		delete(proxy.ServiceName2SvcChain, event.Key)
	}
}

func (proxy *KubeProxy) Register() {
	proxy.ServiceInformer.AddEventHandler(tool.Added, proxy.updateRuntimeServiceHandler)
	proxy.ServiceInformer.AddEventHandler(tool.Modified, proxy.updateRuntimeServiceHandler)
	proxy.ServiceInformer.AddEventHandler(tool.Deleted, proxy.deleteRuntimeServiceHandler)
}

func (proxy *KubeProxy) Run() {
	Init()                                // init chains....
	go proxy.ServiceInformer.Run()        // start the service watch
	go proxy.DNSManager.DNSInformer.Run() // start the dns watch
}
