package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/logging"
)

var (
	BinNamePlugin = filepath.Base(os.Args[0])
)

const (
	// by default, k8s pod's first NIC is eth0
	K8sFirstNICName         = "eth0"
	DefaultUnderlayVethName = "veth0"

	DefaultHostRulePriority = 1000

	IPCheckerRetry    = 3
	IPCheckerInterval = "3s"
	IPCheckerTimeOut  = "3s"
)

type Config struct {
	cnitypes.NetConf

	IPConflict bool `json:"detectIPConflict"`

	LogLevel          logging.LogLevel `json:"log_level"`
	LogOutputFilePath string           `json:"log_output_file_path"`

	CoordinatorUnixSocketPath string `json:"coordinator_unix_socket_path"`
}

func ParseConfig(stdin []byte) (*Config, error) {
	conf := Config{}
	if err := json.Unmarshal(stdin, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return nil, fmt.Errorf("failed to parse prevResult: %v", err)
	}

	if len(conf.CoordinatorUnixSocketPath) == 0 {
		conf.CoordinatorUnixSocketPath = constant.DefaultUnixSocketPath
	}

	if len(conf.LogLevel) == 0 {
		conf.LogLevel = logging.LogLevelDebug
	}

	if len(conf.LogOutputFilePath) == 0 {
		conf.LogOutputFilePath = logging.DefaultCoordinatorPluginLogFilePath
	}

	return &conf, nil
}
