%跨平台编译%
set GOOS=linux
set GOARCH=amd64
go build -o ../../build/createPodsExample ../../cmd/kubelet/dockerClient/createPodsExample.go
go build -o ../../build/mountExample ../../cmd/kubelet/dockerClient/mountExample.go