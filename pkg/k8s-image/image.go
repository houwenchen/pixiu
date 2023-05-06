package image

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caoyingjunz/pixiulib/exec"
)

type kubeReleaseInfo struct {
	// kubernetes version
	// eg: v1.23.0
	kubeVersion string
	// kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy, etcd, pause
	subUnitVersions map[string]string

	// 存放 image 的 dockerhub 地址
	remoteRegistry string
	// 拉取 image 的地址
	sourceRegistry string

	// 当环境没有安装 kubeadm 时，从 kubernetes 的 constants 文件中解析版本
	constantsUrl string
	existKubeadm bool
	existDocker  bool

	// 主机上执行命令的接口
	exec exec.Interface
}

type kubeadmResp struct {
	Kind       string   `json:"kind"`
	ApiVersion string   `json:"apiVersion"`
	Images     []string `json:"images"`
}

// 初始化kubeReleaseInfo
func NewKubeReleaseInfo(releaseBranch string) *kubeReleaseInfo {
	kr := &kubeReleaseInfo{
		kubeVersion:     releaseBranch,
		subUnitVersions: make(map[string]string),
		remoteRegistry:  "wenchenhou",
		sourceRegistry:  "registry.cn-hangzhou.aliyuncs.com/google_containers",
		constantsUrl:    fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kubernetes/%s/cmd/kubeadm/app/constants/constants.go", releaseBranch),
		exec:            exec.New(),
	}

	kr.formatKubeVersion()
	kr.dockerExist()
	kr.kubeadmExist()
	kr.getSubUnitVersions()

	return kr
}

// image 转存的主要逻辑
// 镜像检查，镜像下载，修改 tag，镜像 push
func (kr *kubeReleaseInfo) Process() {
	kr.checkDockerHub()
	kr.pullFromSourceRegistry()
	kr.reTag()
	kr.pushToRemoteRegistry()
}

// kubeVersion 格式检查，标准格式是：v1.23.0
func (kr *kubeReleaseInfo) formatKubeVersion() {
	// TODO:
}

// 检查主机是否安装了 kubeadm
func (kr *kubeReleaseInfo) kubeadmExist() {
	_, err := kr.exec.LookPath("kubeadm")
	if err != nil {
		kr.existKubeadm = false
		return
	}
	kr.existKubeadm = true
}

// 检查主机是否安装了 docker, 直接使用 docker search 命令是否成功判断是否安装 docker，顺便测试与 dockerhub 的连通性
func (kr *kubeReleaseInfo) dockerExist() {
	_, err := kr.exec.Command("docker", "search", "busybox").CombinedOutput()
	if err != nil {
		kr.existDocker = false
		fmt.Println("host docker env have some issue, please check")
		panic(err)
	}
	kr.existDocker = true
}

// 使用不同的方法获取 subUnitVersions
func (kr *kubeReleaseInfo) getSubUnitVersions() {
	if kr.existKubeadm {
		kr.getSubUnitVersionsViaKubeadm()
	} else {
		kr.getSubUnitVersionsViaConstantsUrl()
	}
}

// 使用 kubeadm 构造 subUnitVersions
func (kr *kubeReleaseInfo) getSubUnitVersionsViaKubeadm() error {
	kubeadmresp := &kubeadmResp{}

	out, err := kr.exec.Command("kubeadm", "config", "images", "list", "--kubernetes-version=v1.23.0", "-o=json").CombinedOutput()
	if err != nil {
		// TODO：考虑是否掉入使用 constantsUrl 来解析出版本
		fmt.Println("get subUnitVersions via kubeadm failed")
		fmt.Println(err)
		return err
	}

	err = json.Unmarshal(out, kubeadmresp)
	if err != nil {
		fmt.Println("kubeadmresp unmarshal failed")
		fmt.Println(err)
		return err
	}

	// 对 images 进行处理, 将数据整合进 kubeReleaseInfo 的 subUnitVersions 字段
	for _, image := range kubeadmresp.Images {
		unitVersion := strings.Split(image, "/")
		newUnitVersion := unitVersion[len(unitVersion)-1]
		unitVersion = strings.Split(newUnitVersion, ":")
		kr.subUnitVersions[unitVersion[0]] = unitVersion[1]
	}

	return nil
}

// 使用 constantsUrl 构造 subUnitVersions
func (kr *kubeReleaseInfo) getSubUnitVersionsViaConstantsUrl() error {
	// TODO:
	return nil
}

// 在做镜像转存前，先检查 dockerhub 中是否已经存在镜像
func (kr *kubeReleaseInfo) checkDockerHub() bool {
	// TODO:
	return false
}

func (kr *kubeReleaseInfo) pullFromSourceRegistry() {
	for unit, version := range kr.subUnitVersions {
		// 构造 docker image pull 的格式
		// docker image pull registry.cn-hangzhou.aliyuncs.com/google_containers/ingress-nginx/controller:v1.1.1
		fmt.Println(unit, version)
	}
}

func (kr *kubeReleaseInfo) reTag() {

}

func (kr *kubeReleaseInfo) pushToRemoteRegistry() {

}
