package avail

import (
	"time"

	flag "github.com/spf13/pflag"
)

type DAConfig struct {
	Enable           bool          `koanf:"enable"`
	AvailApiURL      string        `koanf:"avail-api-url"`
	Seed             string        `koanf:"seed"`
	AppID            int           `koanf:"app-id"`
	BridgeApiBaseURL string        `koaf:"bridge-api-base-url"`
	Timeout          time.Duration `koanf:"timeout"`
	VectorX          string        `koanf:"vectorx"`
	ArbSepoliaRPC    string        `koanf:"arbsepolia-rpc"`
}

var DefaultAvailDAConfig = DAConfig{
	Enable:           false,
	AvailApiURL:      "wss://turing-rpc.avail.so/ws",
	Seed:             "",
	AppID:            1,
	BridgeApiBaseURL: "https://turing-bridge-api.fra.avail.so/",
	Timeout:          100 * time.Second,
	VectorX:          "0xA712dfec48AF3a78419A8FF90fE8f97Ae74680F0",
	ArbSepoliaRPC:    "wss://sepolia-rollup.arbitrum.io/rpc",
}

func NewDAConfig(avail_api_url string, seed string, app_id int, bridgeApiBaseURL string, timeout time.Duration, vectorx string, arbSepolia_rpc string) (*DAConfig, error) {
	return &DAConfig{
		Enable:           true,
		AvailApiURL:      avail_api_url,
		Seed:             seed,
		AppID:            app_id,
		BridgeApiBaseURL: bridgeApiBaseURL,
		Timeout:          timeout,
		VectorX:          vectorx,
		ArbSepoliaRPC:    arbSepolia_rpc,
	}, nil
}

func AvailDAConfigAddNodeOptions(prefix string, f *flag.FlagSet) {
	f.Bool(prefix+".enable", DefaultAvailDAConfig.Enable, "enable AvailDA as Data Availability Layer")
	f.String(prefix+".avail-api-url", DefaultAvailDAConfig.AvailApiURL, "Avail chain API offered over the WS-RPC interface")
	f.String(prefix+".seed", DefaultAvailDAConfig.Seed, "Avail chain wallet seed")
	f.Int(prefix+".app-id", DefaultAvailDAConfig.AppID, "Avail chain account app-id")
	f.String(prefix+".bridge-api-base-url", DefaultAvailDAConfig.BridgeApiBaseURL, "Avail bridge api offered over the HTTP-RPC interface")
	f.Duration(prefix+".timeout", DefaultAvailDAConfig.Timeout, "timeout for batch inclusion on block finalisation over Avail chain")
	f.String(prefix+".vectorx", DefaultAvailDAConfig.VectorX, "vectorx contract address for event notification")
	f.String(prefix+".arbsepolia-rpc", DefaultAvailDAConfig.ArbSepoliaRPC, "ArbSepolia api offered over the WS-RPC interface")
}
