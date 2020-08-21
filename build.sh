export CGO_ENABLED=0
go build -o package/dingtalk-alert-server
cd package/ || exit
docker build -t registry.cn-hangzhou.aliyuncs.com/link-cloud/dingtalk-server:hcaas .