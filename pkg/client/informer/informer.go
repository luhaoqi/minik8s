package informer

import (
	"minik8s/pkg/client/tool"
	"sync"
)

type EventHandler func(event tool.Event)

type Informer interface {
	AddEventHandler(etype tool.EventType, handler EventHandler)
	List() []tool.ListRes
	Run()
	Stop()
	Get(key string) string
	Set(key string, val string)
	GetCache() *map[string]string
	Delete(key string)
}

type informer struct {
	stop     bool
	resource string
	lw       tool.ListWatcher
	handlers map[tool.EventType]EventHandler
	Cache    map[string]string
	mutex    sync.Mutex
}

func (i *informer) Get(key string) string {
	return i.Cache[key]
}

func (i *informer) Set(key string, val string) {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	i.Cache[key] = val
}

func (i *informer) Delete(key string) {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	delete(i.Cache, key)
}

func (i *informer) GetCache() *map[string]string {
	return &i.Cache
}

func (i *informer) AddEventHandler(etype tool.EventType, handler EventHandler) {
	i.handlers[etype] = handler
}

func (i *informer) List() []tool.ListRes {
	return i.lw.List(i.resource)
}

func (i *informer) Run() {
	i.stop = false
	watcher := i.lw.Watch(i.resource)
	for {
		select {
		case event := <-watcher.ResultChan():
			if i.stop {
				i.lw.Watch(i.resource).Stop()
				return
			}
			if handler, ok := i.handlers[event.Type]; ok {
				handler(event)
			}
		}
	}
}

func (i *informer) Stop() {
	i.stop = true
}

func NewInformer(resource string) Informer {
	i := informer{
		stop:     false,
		resource: resource,
		lw:       tool.NewListWatchFromClient(resource),
		handlers: make(map[tool.EventType]EventHandler, 4),
		Cache:    make(map[string]string),
	}
	res := i.List()
	for _, v := range res {
		i.Cache[v.Key] = v.Value
	}
	return &i
}
