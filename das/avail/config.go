package avail

import (
	"time"

	"github.com/offchainlabs/nitro/das/avail/availDABridge"
	s3_storage_service "github.com/offchainlabs/nitro/das/avail/s3StorageService"
	flag "github.com/spf13/pflag"
)

type DAConfig struct {
	// Avail Client
	Enable      bool          `koanf:"enable"`
	AvailApiURL string        `koanf:"avail-api-url"`
	Seed        string        `koanf:"seed"`
	AppID       int           `koanf:"app-id"`
	Timeout     time.Duration `koanf:"timeout"`

	// Fallback
	FallbackS3ServiceConfig s3_storage_service.S3StorageServiceConfig `koanf:"fallback-s3-service-config"`
	// AvailDABridge
	AvailBridgeConfig availDABridge.AvailDABridgeConfig `koanf:"avail-bridge-config"`
}

var DefaultAvailDAConfig = DAConfig{
	Enable:                  false,
	AvailApiURL:             "wss://turing-rpc.avail.so/ws",
	Seed:                    "",
	AppID:                   1,
	Timeout:                 100 * time.Second,
	FallbackS3ServiceConfig: s3_storage_service.DefaultS3StorageServiceConfig,
	AvailBridgeConfig:       availDABridge.DefaultAvailDABridgeConfig,
}

// func NewDAConfig(avail_api_url string, seed string, app_id int, bridgeApiBaseURL string, timeout time.Duration, vectorx string, arbSepolia_rpc string) (*DAConfig, error) {
// 	return &DAConfig{
// 		Enable:           true,
// 		AvailApiURL:      avail_api_url,
// 		Seed:             seed,
// 		AppID:            app_id,
// 		BridgeApiBaseURL: bridgeApiBaseURL,
// 		Timeout:          timeout,
// 		VectorX:          vectorx,
// 		ArbSepoliaRPC:    arbSepolia_rpc,
// 	}, nil
// }

func AvailDAConfigAddNodeOptions(prefix string, f *flag.FlagSet) {
	f.Bool(prefix+".enable", DefaultAvailDAConfig.Enable, "enable AvailDA as Data Availability Layer")
	f.String(prefix+".avail-api-url", DefaultAvailDAConfig.AvailApiURL, "Avail chain API offered over the WS-RPC interface")
	f.String(prefix+".seed", DefaultAvailDAConfig.Seed, "Avail chain wallet seed")
	f.Int(prefix+".app-id", DefaultAvailDAConfig.AppID, "Avail chain account app-id")
	f.Duration(prefix+".timeout", DefaultAvailDAConfig.Timeout, "timeout for batch inclusion on block finalisation over Avail chain")
	s3_storage_service.S3ConfigAddOptions(prefix+".fallback-s3-service-config", f)
	availDABridge.AvailDABridgeAddOptions(prefix+".avail-bridge-config", f)
}
