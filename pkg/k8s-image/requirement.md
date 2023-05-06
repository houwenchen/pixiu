给集群的版本，找到对应的组件的版本，将这些组件的对应版本的镜像从k8s.gcr.io整到dockerhub上
可以偷偷用阿里云的

需要有的功能：
1. 下载镜像，修改tag，push到dockerhub
2. 根据k8s的版本，找到其所需依赖的版本
   看了kubeadm的代码，有两种思路：
   2.1 通过下面的方式，需要环境中安装kubeadm
   eg: 
   [root@VM-12-13-centos lighthouse]# kubeadm config images list --kubernetes-version v1.23.4
    k8s.gcr.io/kube-apiserver:v1.23.4
    k8s.gcr.io/kube-controller-manager:v1.23.4
    k8s.gcr.io/kube-scheduler:v1.23.4
    k8s.gcr.io/kube-proxy:v1.23.4
    k8s.gcr.io/pause:3.6
    k8s.gcr.io/etcd:3.5.1-0
    k8s.gcr.io/coredns/coredns:v1.8.6
    Choose a specific Kubernetes version for the control plane. (default "stable-1")

    root@master:~# kubeadm config images list --kubernetes-version v1.23.4 -o json
    {
    "kind": "Images",
    "apiVersion": "output.kubeadm.k8s.io/v1alpha2",
    "images": [
        "k8s.gcr.io/kube-apiserver:v1.23.4",
        "k8s.gcr.io/kube-controller-manager:v1.23.4",
        "k8s.gcr.io/kube-scheduler:v1.23.4",
        "k8s.gcr.io/kube-proxy:v1.23.4",
        "k8s.gcr.io/pause:3.6",
        "k8s.gcr.io/etcd:3.5.1-0",
        "k8s.gcr.io/coredns/coredns:v1.8.6"
    ]
    }
    2.2 使用解析的方法解析出所需的版本--用这种办法吧，高级点
3. 获取所需要deploy的k8s的版本--传入即可
4. 检查dockerhub是否存在镜像
    root@master:~# docker search wenchenhou/busybox
    NAME                 DESCRIPTION   STARS     OFFICIAL   AUTOMATED
    wenchenhou/busybox                 0                    
