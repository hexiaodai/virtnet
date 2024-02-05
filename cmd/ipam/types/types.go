package types

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	cnispecversion "github.com/containernetworking/cni/pkg/version"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/logging"
)

var (
	BinNamePlugin = filepath.Base(os.Args[0])
)

type K8sArgs struct {
	types.CommonArgs
	IP                         net.IP
	K8S_POD_NAME               types.UnmarshallableString //revive:disable-line
	K8S_POD_NAMESPACE          types.UnmarshallableString //revive:disable-line
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString //revive:disable-line
	K8S_POD_UID                types.UnmarshallableString //revive:disable-line
}

type NetConf struct {
	Name       string     `json:"name"`
	CNIVersion string     `json:"cniVersion"`
	IPAM       IPAMConfig `json:"ipam"`
}

type IPAMConfig struct {
	Type string `json:"type"`

	DefaultSubnet string `json:"default_subnet"`

	LogLevel          logging.LogLevel `json:"log_level"`
	LogOutputFilePath string           `json:"log_output_file_path"`

	IPAMUnixSocketPath string `json:"ipam_unix_socket_path"`
}

func LoadNetConf(argsStdin []byte) (*NetConf, error) {
	netConf := &NetConf{}

	if err := json.Unmarshal(argsStdin, netConf); err != nil {
		return nil, fmt.Errorf("failed to parse CNI network configuration: %v", err)
	}

	if len(netConf.IPAM.IPAMUnixSocketPath) == 0 {
		netConf.IPAM.IPAMUnixSocketPath = constant.DefaultUnixSocketPath
	}

	if len(netConf.IPAM.LogLevel) == 0 {
		netConf.IPAM.LogLevel = logging.LogLevelInfo
	}

	if len(netConf.IPAM.LogOutputFilePath) == 0 {
		netConf.IPAM.LogOutputFilePath = logging.DefaultIPAMPluginLogFilePath
	}

	for _, vers := range cnispecversion.All.SupportedVersions() {
		if netConf.CNIVersion == vers {
			return netConf, nil
		}
	}

	return nil, fmt.Errorf("unsupported specified CNI version %s, the CNI versions supported by VirtNet: %v", netConf.CNIVersion, cnispecversion.All.SupportedVersions())
}
