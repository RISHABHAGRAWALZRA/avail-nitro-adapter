package availDABridge

import (
	"context"
	"strings"

	"fmt"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
)

const VectorxABI = `[
    {
        "type": "event",
        "name": "HeadUpdate",
        "inputs": [
            {
                "name": "blockNumber",
                "type": "uint32",
                "indexed": false,
                "internalType": "uint32"
            },
            {
                "name": "headerHash",
                "type": "bytes32",
                "indexed": false,
                "internalType": "bytes32"
            }
        ],
        "anonymous": false
    }
]`

type AvailDABridgeConfig struct {
	VectorXAddress    string `koanf:"vectorx-address"`
	AvailBridgeApiURL string `koanf:"avail-bridge-api-url"`
	ArbSepoliaRPC     string `koanf:"arbsepolia-rpc"`
}

var DefaultAvailDABridgeConfig = AvailDABridgeConfig{
	AvailBridgeApiURL: "https://turing-bridge-api.fra.avail.so/",
	VectorXAddress:    "0xA712dfec48AF3a78419A8FF90fE8f97Ae74680F0",
	ArbSepoliaRPC:     "wss://sepolia-rollup.arbitrum.io/rpc",
}

func AvailDABridgeAddOptions(prefix string, f *flag.FlagSet) {
	f.String(prefix+".avail-bridge-api-url", DefaultAvailDABridgeConfig.AvailBridgeApiURL, "Avail bridge api offered over the HTTP-RPC interface")
	f.String(prefix+".vectorx-address", DefaultAvailDABridgeConfig.VectorXAddress, "vectorx contract address for event notification")
	f.String(prefix+".arbsepolia-rpc", DefaultAvailDABridgeConfig.ArbSepoliaRPC, "ArbSepolia api offered over the WS-RPC interface")
}

type AvailDABridge struct {
	abi                   abi.ABI
	client                *ethclient.Client
	query                 ethereum.FilterQuery
	availbridgeApiBaseURL string
	bridgeApiTimeout      time.Duration
	vectorXTimeout        time.Duration
}

func InitializeAvailDABridge(cfg AvailDABridgeConfig, bridgeApiTimeout, vectorXTimeout time.Duration) (*AvailDABridge, error) {
	// Contract address
	contractAddress := common.HexToAddress(cfg.VectorXAddress)

	// Parse the contract ABI
	abi, err := abi.JSON(strings.NewReader(VectorxABI))
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot create abi for vectorX, %w", err)
	}

	// Connect to L1 node thru web socket
	client, err := ethclient.Dial(cfg.ArbSepoliaRPC)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: %w", err)
	}

	// Create a filter query to listen for events
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddress},
		Topics:    [][]common.Hash{{abi.Events["HeadUpdate"].ID}},
	}

	return &AvailDABridge{
		abi:                   abi,
		client:                client,
		query:                 query,
		availbridgeApiBaseURL: cfg.AvailBridgeApiURL,
		bridgeApiTimeout:      bridgeApiTimeout,
		vectorXTimeout:        vectorXTimeout,
	}, nil
}

func (b *AvailDABridge) SubscribeForHeaderUpdate(finalizedBlockNumber uint32, t time.Duration) error {
	// Subscribe to the event stream
	logs := make(chan types.Log)
	sub, err := b.client.SubscribeFilterLogs(context.Background(), b.query, logs)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	// Keep the connection alive with a ping mechanism
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	go func(t *time.Ticker) {
		for range t.C {
			// Send a dummy request to keep the connection alive
			_, err := b.client.BlockNumber(context.Background())
			if err != nil {
				log.Warn("Failed to send ping to keep the connection alive for vectorx events", "err", err)
			}
		}
	}(ticker)

	log.Info("🎧  Listening for vectorx HeadUpdate event with", "blockNumber", finalizedBlockNumber)
	timeout := time.After(t * time.Second)
	// Loop to process incoming events
	for {
		select {
		case err := <-sub.Err():
			return err
		case vLog := <-logs:
			event, err := b.abi.Unpack("HeadUpdate", vLog.Data)
			if err != nil {
				return err
			}

			log.Info("🤝  New HeadUpdate event from vecotorx", "blockNumber", event[0])
			val, _ := event[0].(uint32)
			if val >= uint32(finalizedBlockNumber) {
				return nil
			}
		case <-timeout:
			return fmt.Errorf("⌛️  Timeout of %d seconds reached without getting HeadUpdate event from vectorx for blockNumber %v", t, finalizedBlockNumber)
		}
	}
}
