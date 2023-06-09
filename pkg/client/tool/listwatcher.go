package tool

type WatchInterface interface {
	// Stop stops watching.
	Stop()

	// ResultChan returns a chan which will receive all the events.
	ResultChan() <-chan Event
}

type watcher struct {
	resultChan chan Event
}

func (w *watcher) Stop() {
	close(w.resultChan)
}

func (w *watcher) ResultChan() <-chan Event {
	return w.resultChan
}

type ListWatcher interface {
	// List should return a resource type specific collection of items.
	List(resource string) []ListRes
	// Watch should return a resource type specific watch.Interface that watches for changes to items.
	Watch(resourceVersion string) WatchInterface
}

type listwatcher struct {
	ListFunc  func(resource string) []ListRes
	WatchFunc func(resourceVersion string) WatchInterface
}

func (lw *listwatcher) List(resource string) []ListRes {
	return lw.ListFunc(resource)
}

func (lw *listwatcher) Watch(resourceVersion string) WatchInterface {
	return lw.WatchFunc(resourceVersion)
}

// NewListWatchFromClient creates a ListWatch from the specified client, resource, namespace and field selector.
func NewListWatchFromClient(resource string) ListWatcher {
	return &listwatcher{
		ListFunc: func(resource string) []ListRes {
			return List(resource)
		},
		WatchFunc: func(resource string) WatchInterface {
			return Watch(resource)
		},
	}
}
