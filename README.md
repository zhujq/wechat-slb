# wechat-slb
wechat-slb，简易wechat后端负载均衡器，是wechat后端面向腾讯服务器的入口，通过负载均衡反向代理到各后端服务器（包括vps部署和paas部署），配置文件为slb.json
dockerfile用于生成镜像，镜像名为zhujq/ubuntu:svslb
