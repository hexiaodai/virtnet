package main

import (
	"coo/logging"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containernetworking/cni/pkg/types"
)

func main() {
	stdinData := `
	{
		"type": "virtnest-coordinator"
	}
	`
	netConf := types.NetConf{}
	if err := json.Unmarshal([]byte(stdinData), &netConf); err != nil {
		fmt.Printf("failed to parse config: %v\n", err)
	}

	logger := logging.LoggerFromOutputFile(logging.LogLevelDebug, logging.DefaultCooFilePath).Named("main")

	logger.Debugw("Debug message", "netConf", netConf, "args", os.Args)
}
