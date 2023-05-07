package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/caoyingjunz/pixiulib/exec"
	"k8s.io/apimachinery/pkg/util/version"
)

const (
	remoteRegistryUrl string = "wenchenhou"
	sourceRegistryUrl string = "registry.cn-hangzhou.aliyuncs.com/google_containers"
)

type kubeReleaseInfo struct {
	// kubernetes version
	// eg: v1.23.0
	kubeVersion string
	// kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy, etcd, pause, coredns
	subUnitVersions map[string]string
	// 记录 kubeadm 获取组件版本信息时的镜像的前缀
	subUnitPrefixs map[string]string
	// 记录 kubernetes 集群的各个组件的 image 是否在 dockerhub 中存在
	subUnitExist map[string]bool

	// 存放 image 的 dockerhub 地址
	remoteRegistry  string
	remoteImageInfo map[string]string
	// 拉取 image 的地址
	sourceRegistry  string
	sourceImageInfo map[string]string

	// 当环境没有安装 kubeadm 时，从 kubernetes 的 constants 文件中解析版本
	constantsUrl string
	existKubeadm bool
	existDocker  bool

	// 主机上执行命令的接口
	exec exec.Interface
}

type writeCounter struct {
	total       int64
	totalLength int64
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
		subUnitPrefixs:  make(map[string]string),
		subUnitExist:    make(map[string]bool),
		remoteRegistry:  remoteRegistryUrl,
		remoteImageInfo: make(map[string]string),
		sourceRegistry:  sourceRegistryUrl,
		sourceImageInfo: make(map[string]string),
		constantsUrl:    fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kubernetes/%s/cmd/kubeadm/app/constants/constants.go", releaseBranch),
		exec:            exec.New(),
	}

	kr.formatKubeVersion()
	kr.dockerExist()
	kr.kubeadmExist()
	kr.getSubUnitVersions()
	kr.buildAllImageInfo()
	kr.checkDockerHub()

	return kr
}

func (kr *kubeReleaseInfo) Run() {
	kr.imageManageProcess()
}

// kubeVersion 格式检查，标准格式是：v1.23.0
func (kr *kubeReleaseInfo) formatKubeVersion() {
	// TODO:
}

// 检查主机是否安装了 docker, 直接使用 docker search 命令是否成功判断是否安装 docker，顺便测试与 dockerhub 的连通性
func (kr *kubeReleaseInfo) dockerExist() {
	_, err := kr.exec.Command("docker", "search", "busybox").CombinedOutput()
	if err != nil {
		kr.existDocker = false
		fmt.Println("host docker env have some issue, please check")
		// panic(err)
	}
	kr.existDocker = true
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
	// k8s.gcr.io/coredns/coredns:v1.8.6
	for _, image := range kubeadmresp.Images {
		unitInfos := strings.Split(image, "/")
		prefix := strings.Join(unitInfos[:len(unitInfos)-1], "/")
		UnitAndVersion := unitInfos[len(unitInfos)-1]
		unitVersion := strings.Split(UnitAndVersion, ":")
		kr.subUnitPrefixs[unitVersion[0]] = prefix
		kr.subUnitVersions[unitVersion[0]] = unitVersion[1]
	}

	return nil
}

// 使用 constantsUrl 构造 subUnitVersions
func (kr *kubeReleaseInfo) getSubUnitVersionsViaConstantsUrl() error {
	infos := make(map[string]string)
	v, _ := version.ParseGeneric(kr.kubeVersion)
	fmt.Println(v)
	err := kr.getImageVersions(v, infos)
	if err != nil {
		return err
	}
	delete(infos, "conformance")
	if version, ok := infos["coredns/coredns"]; ok {
		delete(infos, "coredns/coredns")
		infos["coredns"] = version
	}
	kr.subUnitVersions = infos
	return nil
}

// parse the kubeadm config and obtain image versions that are not bound to the k8s version.
func (kr *kubeReleaseInfo) getImageVersions(ver *version.Version, images map[string]string) error {
	constants, _, err := kr.getFromURL()
	// 这里不清楚为啥不加打印就会报错
	fmt.Println(constants)
	fmt.Println(err)
	if err != nil {
		return err
	}
	lines := strings.Split(constants, "\n")

	// create a map of all required images
	// map[coredns:v1.8.6 etcd:3.5.1-0 kube-apiserver:v1.23.0 kube-controller-manager:v1.23.0
	// kube-proxy:v1.23.0 kube-scheduler:v1.23.0 pause:3.6]
	k8sVersionV := "v" + ver.String()
	images["kube-apiserver"] = k8sVersionV
	images["kube-controller-manager"] = k8sVersionV
	images["kube-scheduler"] = k8sVersionV
	images["kube-proxy"] = k8sVersionV
	images["etcd"] = ""
	images["pause"] = ""

	// images outside the scope of kubeadm, but still using the k8s version

	// the hyperkube image was removed for version v1.17
	if ver.Major() == 1 && ver.Minor() < 17 {
		images["hyperkube"] = k8sVersionV
	}
	// the cloud-controller-manager image was removed for version v1.16
	if ver.Major() == 1 && ver.Minor() < 16 {
		images["cloud-controller-manager"] = k8sVersionV
	}
	// test the conformance image, but only for newer versions as it was added in v1.13.0-alpha.2
	// also skip v1.21.0-beta.1 due to a bug that caused this image tag to not be released.
	conformanceMinVer := version.MustParseSemantic("v1.13.0-alpha.2")
	is21beta1, _ := ver.Compare("v1.21.0-beta.1")
	if ver.AtLeast(conformanceMinVer) && is21beta1 != 0 {
		images["conformance"] = k8sVersionV
	}

	// coredns changed image location after 1.21.0-alpha.1
	coreDNSNewVer := version.MustParseSemantic("v1.21.0-alpha.1")
	coreDNSPath := "coredns"
	if ver.AtLeast(coreDNSNewVer) {
		coreDNSPath = "coredns/coredns"
	}

	// parse the constants file and fetch versions.
	// note: Split(...)[1] is safe here given a line contains the key.
	for _, line := range lines {
		if strings.Contains(line, "CoreDNSVersion = ") {
			line = strings.TrimSpace(line)
			line = strings.Split(line, "CoreDNSVersion = ")[1]
			line = strings.Replace(line, `"`, "", -1)
			images[coreDNSPath] = line
		} else if strings.Contains(line, "DefaultEtcdVersion = ") {
			line = strings.TrimSpace(line)
			line = strings.Split(line, "DefaultEtcdVersion = ")[1]
			line = strings.Replace(line, `"`, "", -1)
			images["etcd"] = line
		} else if strings.Contains(line, "PauseVersion = ") {
			line = strings.TrimSpace(line)
			line = strings.Split(line, "PauseVersion = ")[1]
			line = strings.Replace(line, `"`, "", -1)
			images["pause"] = line
		}
	}
	// hardcode the tag for pause as older k8s branches lack a constant.
	if images["pause"] == "" {
		images["pause"] = "3.1"
	}
	// verify.
	fmt.Printf("* getImageVersions(): [%s] %#v\n", ver.String(), images)
	if images[coreDNSPath] == "" || images["etcd"] == "" {
		return fmt.Errorf("at least one image version could not be set: %#v", images)
	}
	return nil
}

func (wc *writeCounter) PrintProgress() {
	if wc.totalLength == 0 {
		fmt.Printf("\r* progress...%d bytes", wc.total)
		return
	}
	fmt.Printf("\r* progress...%d %% ", int64((float64(wc.total)/float64(wc.totalLength))*100.0))
}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.total += int64(n)
	return n, nil
}

// downloads the contents of a web page into a string.
// use default timeout of 10 seconds.
func (kr *kubeReleaseInfo) getFromURL() (string, int, error) {
	url := kr.constantsUrl
	t := time.Duration(time.Duration(10) * time.Second)
	client := http.Client{
		Timeout: t,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", -1, err
	}
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", -1, fmt.Errorf("responded with status: %d", resp.StatusCode)
	}

	len := resp.Header.Get("Content-Length")
	sz, err := strconv.Atoi(len)
	if err != nil {
		sz = 0
	}

	var src io.Reader
	var dst bytes.Buffer

	counter := &writeCounter{totalLength: int64(sz)}
	src = io.TeeReader(resp.Body, counter)

	_, err = io.Copy(&dst, src)
	if err != nil {
		panic(err)
	}

	return dst.String(), sz, nil
}

// 维护 remoteImageInfo 和 sourceImageInfo 字段
func (kr *kubeReleaseInfo) buildAllImageInfo() {
	// 以组件 coredns 为例
	// remoteImageInfo 中：wenchenhou/coredns:v1.8.6
	// sourceImageInfo 中: registry.cn-hangzhou.aliyuncs.com/google_containers/coredns:v1.8.6
	for unitName, unitVersion := range kr.subUnitVersions {
		kr.remoteImageInfo[unitName] = kr.remoteRegistry + "/" + unitName + ":" + unitVersion
		kr.sourceImageInfo[unitName] = kr.sourceRegistry + "/" + unitName + ":" + unitVersion
	}
}

// 在做镜像转存前，先检查 dockerhub 中是否已经存在镜像
// 将检查的结果维护在 subUnitExist 字段中
// 因为 docker search 没有办法获取 image 的 tag 信息
// 所以使用 docker pull 的返回来判断 image 是否存在
// 维护 subUnitExist 字段
// TODO：后面的三个函数是否执行可以依赖这里维护的字段
func (kr *kubeReleaseInfo) checkDockerHub() {
	for unitName, unitInfo := range kr.remoteImageInfo {
		// docker image pull wenchenhou/coredns:v1.8.6
		// TODO: 本地存在没有 push 上去的情况需要考虑下
		_, err := kr.exec.Command("docker", "image", "pull", unitInfo).CombinedOutput()
		if err != nil {
			kr.subUnitExist[unitName] = false
			continue
		}
		kr.subUnitExist[unitName] = true
	}
}

// 实现镜像下载，修改 tag ，转存到dockerhub
// TODO: 需要增加 handleErr 的逻辑
// 这个逻辑需要考虑的比较多，介入的时间点，以及重做的位置的定位
// 思路：开启一个死循环，以是否所有的操作均完成为判断标准，每个操作 err 的时候就会有一个信号产生
func (kr *kubeReleaseInfo) imageManageProcess() {
	for unitName, exist := range kr.subUnitExist {
		if !exist {
			pullErr := kr.pullFromSourceRegistry(unitName)
			if pullErr != nil {
				// TODO:
				fmt.Println()
			}
			retagErr := kr.retagImage(unitName)
			if retagErr != nil {
				// TODO:
				fmt.Println()
			}
			pushErr := kr.pushToRemoteRegistry(unitName)
			if pushErr != nil {
				// TODO:
				fmt.Println()
			}
		}
	}
}

func (kr *kubeReleaseInfo) pullFromSourceRegistry(unitName string) error {
	// docker image pull registry.cn-hangzhou.aliyuncs.com/google_containers/ingress-nginx/controller:v1.1.1
	out, err := kr.exec.Command("docker", "image", "pull", kr.sourceImageInfo[unitName]).CombinedOutput()
	if err != nil {
		fmt.Println("docker image pull failed, err: ", err)
		return err
	}
	fmt.Println(string(out))
	return nil
}

func (kr *kubeReleaseInfo) retagImage(unitName string) error {
	// docker image tag registry.cn-hangzhou.aliyuncs.com/google_containers/coredns:v1.8.6 wenchenhou/coredns:v1.8.6
	_, err := kr.exec.Command("docker", "image", "tag", kr.sourceImageInfo[unitName], kr.remoteImageInfo[unitName]).CombinedOutput()
	if err != nil {
		fmt.Println("docker image tag failed, err: ", err)
		return err
	}
	return nil
}

func (kr *kubeReleaseInfo) pushToRemoteRegistry(unitName string) error {
	// docker image push wenchenhou/coredns:v1.8.6
	out, err := kr.exec.Command("docker", "image", "push", kr.remoteImageInfo[unitName]).CombinedOutput()
	if err != nil {
		fmt.Println("docker image push failed, err: ", err)
		return err
	}
	fmt.Println(string(out))
	return nil
}
