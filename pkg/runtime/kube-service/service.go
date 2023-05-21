package kube_service

import (
	"encoding/json"
	"errors"
	"fmt"
	"minik8s/cmd/kube-apiserver/app/apiconfig"
	"minik8s/pkg/api/core"
	"minik8s/pkg/client/informer"
	"minik8s/pkg/client/tool"
	"minik8s/pkg/service"
	"sync"
	"time"
)

// findPods
// runtimeService根据seletors的信息找到
func (rs *RuntimeService) findPods(isCreate bool) error {
	prefix := "[service][findPods]:"
	fmt.Println(prefix + "in")
	// 使用informer的list方法获取所有的pods，放到str中
	// inspired by add-node.go
	str := rs.Informer.List()
	var curPods []*core.Pod
	// 获取最新的pod消息，放到curPods中
	for _, val := range str {
		tmp := &core.Pod{}
		err := json.Unmarshal([]byte(val.Value), tmp)
		if err != nil {
			continue
		}
		curPods = append(curPods, tmp)
	}
	// 按照selector筛选pods
	var filtedPods []*core.Pod
	for _, pod := range curPods {
		if pod.Status.Phase != core.PodRunning {
			continue
		}
		isChosed := true
		if rs.ServiceConfig.ServiceSpec.Selector != nil {
			for k, v := range rs.ServiceConfig.ServiceSpec.Selector {
				value, ok := pod.Labels[k]
				fmt.Println(prefix + "expect k=" + k + " v=" + v + "av=" + value)
				if !ok || v != value {
					isChosed = false
					break
				}
			}
		}
		if isChosed {
			fmt.Println(prefix + "chosen Pod:" + pod.Name)
			filtedPods = append(filtedPods, pod)
		}
	}
	// 更新pods为最新
	rs.Pods = filtedPods
	// 使用client发送更新的通知给api-server，使其更新etcd
	// 仿照add-node中的addNode方法
	//ifUpdate := false
	var newNameIp []service.PodNameAndIp
	if len(filtedPods) == 0 { // no pod, error
		if isCreate {
			rs.ServiceConfig.ServiceSpec.Status.Phase = service.ServiceErrorPhase
			rs.ServiceConfig.ServiceSpec.Status.Err = "NoOKPodsWhenInit"
		} else {
			rs.ServiceConfig.ServiceSpec.Status.Phase = service.ServiceErrorPhase
			rs.ServiceConfig.ServiceSpec.Status.Err = "NoOKPodsNow"
		}
		//ifUpdate = true
	} else {
		rs.ServiceConfig.ServiceSpec.Status.Phase = service.ServiceRunningPhase
		rs.ServiceConfig.ServiceSpec.Status.Err = ""
		for _, val := range rs.Pods {
			fmt.Println("[service][findPods]: name=" + val.Name + " ip=" + val.Status.PodIP)
			newNameIp = append(newNameIp, service.PodNameAndIp{Name: val.Name, Ip: val.Status.PodIP})
		}
		rs.ServiceConfig.PodNameAndIps = newNameIp
	}
	// update etcd by client method
	err := tool.UpdateService(rs.ServiceConfig)
	return err
}

// 根据spec创建对应的service，绑定handler
func CreateService(sc *service.Service) *RuntimeService {
	res := &RuntimeService{}
	res.ServiceConfig = sc
	res.eventChan = make(chan string)
	//init the pod informer
	res.Informer = informer.NewInformer(apiconfig.POD_PATH)
	var lock sync.RWMutex
	res.lock = lock
	res.ifSend = false
	go res.Run(res.eventChan)
	err := res.findPods(true)
	if err != nil {
		res.ServiceConfig.ServiceSpec.Status.Err = err.Error()
	} else {
		res.ServiceConfig.ServiceSpec.Status.Err = ""
	}
	res.ifSend = true
	// 每10秒更新一下状态
	res.timer = time.NewTicker(10 * time.Second)
	go res.startTicker()
	return res
}

// 运行这个runtimeService
// 时刻根据pod的真实状态更新缓存
func (rs *RuntimeService) Run(event <-chan string) {
	for {
		select {
		case cmd, ok := <-event: // 不阻塞
			if !ok {
				return
			}
			rs.lock.Lock()
			switch cmd {
			case TICK_EVENT:
				// flag表示之前的状态有无改变
				fmt.Println("[service][run]: got ticker")
				flag := false
				for _, pod := range rs.Pods {
					// 应该调用restClient，根据pod的名字查询当前pod的状态
					curPod, err := tool.GetPod(pod.Name)
					if err != nil {
						continue
					}
					if curPod == nil { // pod生命周期结束，需要重新选择
						pod.Status.Phase = core.PodSucceeded
						flag = true
					}
				}
				if flag { //
					fmt.Println("[service][run]: refind pods")
					err := rs.findPods(false)
					if err != nil { // 这里直接终止
						panic(err.Error())
					}
				}
				rs.ifSend = true
			default:
				err := errors.New("NotSupportMethod" + cmd)
				panic(err.Error())
			}
			rs.lock.Unlock()
		}
	}
}

// 一个协程，每10秒发送一个tick, tick会被run捕获从而更新RunService中的状态
func (rs *RuntimeService) startTicker() {
	rs.stopChan = make(chan bool)
	defer rs.timer.Stop()
	for {
		select {
		case <-rs.timer.C:
			rs.lock.Lock()
			if rs.ifSend {
				rs.eventChan <- TICK_EVENT
				// 停止发送tick，直到上一个tick被处理
				rs.ifSend = false
			}
			rs.lock.Unlock()
		case <-rs.stopChan:
			return // stop and return
		}
	}
}

// delete current service
func (rs *RuntimeService) Delete() {
	rs.lock.Lock()
	defer rs.lock.Unlock()
	rs.stopChan <- true // close the ticker
	close(rs.eventChan)
	err := tool.DeleteService(rs.ServiceConfig)
	if err != nil {
		fmt.Println(err.Error())
	}
}
