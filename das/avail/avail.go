package avail

import (
	"context"
	"errors"
	"fmt"
	"math"

	"time"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	gsrpc_types "github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/offchainlabs/nitro/arbutil"
	"github.com/offchainlabs/nitro/das/avail/availDABridge"
	s3_storage_service "github.com/offchainlabs/nitro/das/avail/s3StorageService"
	"github.com/offchainlabs/nitro/das/dastree"
)

const (
	CUSTOM_ARBOSVERSION_AVAIL      = 33
	AvailMessageHeaderFlag    byte = 0x0a
	BridgeApiTimeout               = time.Duration(1200)
	VectorXTimeout                 = time.Duration(10000)
)

var (
	ErrAvailDAClientInit          = errors.New("unable to initialize to connect with AvailDA")
	ErrBatchSubmitToAvailDAFailed = errors.New("unable to submit batch to AvailDA")
	ErrWrongAvailDAPointer        = errors.New("unable to retrieve batch, wrong blobPointer")
)

func IsAvailMessageHeaderByte(header byte) bool {
	return (AvailMessageHeaderFlag & header) > 0
}

type AvailDA struct {
	// Config
	enable              bool
	finalizationTimeout time.Duration
	appID               int

	// Client
	api         *gsrpc.SubstrateAPI
	meta        *gsrpc_types.Metadata
	genesisHash gsrpc_types.Hash
	rv          *gsrpc_types.RuntimeVersion
	keyringPair signature.KeyringPair
	key         gsrpc_types.StorageKey

	// Fallback
	fallbackS3Service *s3_storage_service.S3StorageService
	// AvailDABridge
	availDABridge availDABridge.AvailDABridge
}

func NewAvailDA(cfg DAConfig, l1Client arbutil.L1Interface) (*AvailDA, error) {

	Seed := cfg.Seed
	AppID := cfg.AppID

	appID := 0
	// if app id is greater than 0 then it must be created before submitting data
	if AppID != 0 {
		appID = AppID
	}

	// Creating new substrate api
	api, err := gsrpc.NewSubstrateAPI(cfg.AvailApiURL)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: %w. %w", err, ErrAvailDAClientInit)
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot get metadata, %w. %w", err, ErrAvailDAClientInit)
	}

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot get block hash, %w. %w", err, ErrAvailDAClientInit)
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot get runtime version, %w. %w", err, ErrAvailDAClientInit)
	}

	keyringPair, err := signature.KeyringPairFromSecret(Seed, 42)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot create keyPair, %w. %w", err, ErrAvailDAClientInit)
	}

	key, err := gsrpc_types.CreateStorageKey(meta, "System", "Account", keyringPair.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot create storage key, %w. %w", err, ErrAvailDAClientInit)
	}

	var fallbackS3Service *s3_storage_service.S3StorageService
	if cfg.FallbackS3ServiceConfig.Enable {
		fallbackS3Service, err = s3_storage_service.NewS3StorageService(cfg.FallbackS3ServiceConfig)
		if err != nil {
			return nil, fmt.Errorf("AvailDAError: unable to intialize s3 storage service for fallback, %w. %w", err, ErrAvailDAClientInit)
		}
	}

	return &AvailDA{
		enable:              cfg.Enable,
		finalizationTimeout: time.Duration(cfg.Timeout),
		appID:               appID,
		api:                 api,
		meta:                meta,
		genesisHash:         genesisHash,
		rv:                  rv,
		keyringPair:         keyringPair,
		key:                 key,
		fallbackS3Service:   fallbackS3Service,
		availDABridge:       availDABridge.AvailDABridge{},
	}, nil
}

func (a *AvailDA) Store(ctx context.Context, message []byte) ([]byte, error) {

	finalizedblockHash, nonce, err := submitData(a, message)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: cannot submit data to avail: %w, %w", err, ErrBatchSubmitToAvailDAFailed)
	}

	header, err := a.api.RPC.Chain.GetHeader(finalizedblockHash)
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: cannot get header for finalized block: %w", err)
	}

	extrinsicIndex, err := getExtrinsicIndex(a.api, finalizedblockHash, a.keyringPair.Address, nonce)
	if err != nil {
		return nil, err
	}
	log.Info("AvailDAInfo: 🏆  Data included in Avail's finalised block", "blockHash", finalizedblockHash.Hex(), "extrinsicIndex", extrinsicIndex)

	blobProof, err := queryBlobProof(a.api, extrinsicIndex, finalizedblockHash)
	if err != nil {
		return nil, err
	}

	// validation of blobProof in respect of submitted data
	blobDataKeccak256H := crypto.Keccak256Hash(message)
	if !validateBlobProof(blobProof, blobDataKeccak256H) {
		return nil, fmt.Errorf("AvailDAError: BlobProof is invalid, BlobProof:%s", blobProof.String())
	}

	// Creating BlobPointer to submit over settlement layer
	blobPointer := BlobPointer{Version: BLOBPOINTER_VERSION2, BlockHeight: uint32(header.Number), ExtrinsicIndex: uint32(extrinsicIndex), DasTreeRootHash: dastree.Hash(message), BlobDataKeccak265H: blobDataKeccak256H, BlobProof: blobProof} //nolint: gosec
	log.Info("AvailInfo: ✅  Sucesfully included in block data to Avail", "BlobPointer:", blobPointer.String())
	blobPointerData, err := blobPointer.MarshalToBinary()
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ BlobPointer MashalBinary error, %w", err)
	}

	// fallback
	if a.fallbackS3Service != nil {
		err := a.fallbackS3Service.Put(ctx, message, 0)
		if err != nil {
			log.Error("AvailDAError: failed to put data on s3 storage service: %w", err)
		}
	}

	return blobPointerData, nil
}

func (a *AvailDA) Read(ctx context.Context, blobPointer BlobPointer) ([]byte, error) {
	log.Info("AvailInfo: ℹ️ Requesting data from Avail", "BlobPointer", blobPointer)

	// Intitializing variables
	blockHeight := blobPointer.BlockHeight
	extrinsicIndex := blobPointer.ExtrinsicIndex

	var data []byte
	for i := 0; i < 3; i++ {
		var err error
		data, err = readData(a, blockHeight, extrinsicIndex)
		if err == nil {
			log.Info("AvailInfo: ✅  Succesfully fetched data from Avail")
			break
		} else if i == 2 {
			log.Info("AvailInfo: ❌  failed to fetched data from Avail, err: %w", err)

			if a.fallbackS3Service != nil {
				data, err = a.fallbackS3Service.GetByHash(ctx, blobPointer.DasTreeRootHash)
				if err != nil {
					log.Info("AvailInfo: ❌  failed to read data from fallback s3 storage, err: %w", err)
					return nil, fmt.Errorf("AvailDAError: unable to read data from AvailDA & Fallback s3 storage")
				}
				log.Info("AvailInfo: ✅  Succesfully fetched data from Avail using fallbackS3Service")
				break
			} else {
				return nil, fmt.Errorf("AvailDAError: unable to read data from AvailDA & Fallback s3 storage is not enabled")
			}

		}
		sleepDuration := time.Duration(math.Pow(2, float64(i))) * time.Second
		time.Sleep(sleepDuration)
	}

	return data, nil
}

func readData(a *AvailDA, blockHeight, extrinsicIndex uint32) ([]byte, error) {
	latestHeader, err := a.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: cannot get latest header, %w", err)
	}

	if latestHeader.Number < gsrpc_types.BlockNumber(blockHeight) {
		return nil, fmt.Errorf("AvailDAError: %w: %w", err, ErrWrongAvailDAPointer)
	}

	blockHash, err := a.api.RPC.Chain.GetBlockHash(uint64(blockHeight))
	if err != nil {
		return nil, fmt.Errorf("AvailDAError: ⚠️ cannot get block hash, %w", err)
	}

	// Fetching block based on block hash
	avail_blk, err := a.api.RPC.Chain.GetBlock(blockHash)
	if err != nil {
		return []byte{}, fmt.Errorf("AvailDAError: ❌ cannot get block for hash:%v and getting error:%w", blockHash.Hex(), err)
	}

	// Extracting the required extrinsic according to the reference
	data, err := extractExtrinsicData(avail_blk, extrinsicIndex)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func submitData(a *AvailDA, message []byte) (gsrpc_types.Hash, gsrpc_types.UCompact, error) {
	c, err := gsrpc_types.NewCall(a.meta, "DataAvailability.submit_data", gsrpc_types.NewBytes(message))
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⚠️ cannot create new call, %w", err)
	}

	// Create the extrinsic
	ext := gsrpc_types.NewExtrinsic(c)

	var accountInfo gsrpc_types.AccountInfo
	ok, err := a.api.RPC.State.GetStorageLatest(a.key, &accountInfo)
	if err != nil || !ok {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⚠️ cannot get latest storage, %w", err)
	}

	o := gsrpc_types.SignatureOptions{
		BlockHash:          a.genesisHash,
		Era:                gsrpc_types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        a.genesisHash,
		Nonce:              gsrpc_types.NewUCompactFromUInt(uint64(accountInfo.Nonce)),
		SpecVersion:        a.rv.SpecVersion,
		Tip:                gsrpc_types.NewUCompactFromUInt(0),
		AppID:              gsrpc_types.NewUCompactFromUInt(uint64(a.appID)), //nolint:gosec
		TransactionVersion: a.rv.TransactionVersion,
	}

	// Sign the transaction using Alice's default account
	err = ext.Sign(a.keyringPair, o)
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⚠️ cannot sign, %w", err)
	}

	// Send the extrinsic
	sub, err := a.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⚠️ cannot submit extrinsic, %w", err)
	}

	log.Info("AvailDAInfo: ✅  Tx batch is submitted to Avail", "length", len(message), "address", a.keyringPair.Address, "appID", a.appID)

	defer sub.Unsubscribe()
	timeout := time.After(a.finalizationTimeout * time.Second)
	var finalizedblockHash gsrpc_types.Hash

outer:
	for {
		select {
		case status := <-sub.Chan():
			if status.IsInBlock {
				log.Info("AvailDAInfo: 📥  Submit data extrinsic included in block", "blockHash", status.AsInBlock.Hex())
			} else if status.IsFinalized {
				finalizedblockHash = status.AsFinalized
				log.Info("AvailDAInfo: 📥  Submit data extrinsic included in finalized block", "blockHash", finalizedblockHash.Hex())
				break outer
			} else if status.IsRetracted {
				log.Warn("AvailDAWarn: ✂️  AvailDA transaction got retracted from block", "blockHash", status.AsRetracted.Hex())
			} else if status.IsInvalid {
				return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("❌ Extrinsic invalid")
			}
		case <-timeout:
			return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⌛️  Timeout of %d seconds reached without getting finalized status for extrinsic", a.finalizationTimeout)
		}
	}

	return finalizedblockHash, o.Nonce, nil
}
