package vectorx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/gorilla/websocket"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
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

type VectorX struct {
	Abi    abi.ABI
	Client *ethclient.Client
	Query  ethereum.FilterQuery
}

func (v *VectorX) fetchHeaderUpdateEvent(ctx context.Context, finalizedBlockNumber int) error {
	// Subscribe to the event stream
	logs := make(chan types.Log)
	sub, err := v.Client.SubscribeFilterLogs(ctx, v.Query, logs)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	log.Info("🎧  Listening for vectorx HeadUpdate event with", "blockNumber", finalizedBlockNumber)
	// Loop to process incoming events
	for {
		select {
		case err := <-sub.Err():
			return err
		case vLog := <-logs:
			event, err := v.Abi.Unpack("HeadUpdate", vLog.Data)
			if err != nil {
				return err
			}

			log.Info("🤝  New HeadUpdate event from vecotorx", "blockNumber", event[0])
			val, _ := event[0].(uint32)
			if val >= uint32(finalizedBlockNumber) {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("⌛️  Timeout reached without getting HeadUpdate event from vectorx for blockNumber %v", finalizedBlockNumber)
		}
	}
}

func (v *VectorX) SubscribeForHeaderUpdate(finalizedBlockNumber int, t time.Duration) error {
	retryTimes := 3
	ctx, cancel := context.WithTimeout(context.Background(), t*time.Second)
	defer cancel()

	var err error
	for i := 0; i < retryTimes; i++ {
		log.Warn("Iteration", "Counter", strconv.Itoa(i+1), "Limit", retryTimes)
		err = v.fetchHeaderUpdateEvent(ctx, finalizedBlockNumber)
		if websocket.IsUnexpectedCloseError(err, websocket.CloseAbnormalClosure) {
			log.Warn("Unexpected socket Closure:", err)
			continue
		}
		break
	}

	return err
}
