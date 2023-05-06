package image

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestNewKubeReleaseInfo(t *testing.T) {
	kr := NewKubeReleaseInfo("v1.23.0")
	fmt.Printf("%+v\n", kr)
}

// small test
func Test1(t *testing.T) {
	_, err := exec.LookPath("docker")
	fmt.Println(err)
}
