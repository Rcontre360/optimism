package derive

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
)

var (
	L1WorldcoinRegistry   = common.HexToAddress("0x88a805732825450f4d6e03716Dc60ae206BA1d95")
	RootSentEvent         = "RootSentMultichain(uint256,uint128)"
	RootSentEventABIHash  = crypto.Keccak256Hash([]byte(RootSentEvent))
	RootSentEventVersion0 = common.Hash{}
)

var (
	L2UpdateRootFunction = "receiveRoot(uint256,uint128)"
	L2UpdateArguments    = 2
	L2UpdateLen          = 4 + 32*L2UpdateArguments
)

var (
	L2UpdateFuncBytes4 = crypto.Keccak256([]byte(L2UpdateRootFunction))[:4]
	L2RegistryContract = common.HexToAddress("0x59f7Dd1472c89cb721378073d3662919984D06b2")
)

type L1UpdateInfo struct {
	hash      common.Hash
	timestamp common.Hash
}

func (info *L1UpdateInfo) CallData() ([]byte, error) {
	data := make([]byte, L2UpdateLen)
	offset := 0
	copy(data[offset:4], L2UpdateFuncBytes4)
	offset += 4

	// Append info.hash to data
	copy(data[offset:offset+len(info.hash)], info.hash[:])
	offset += len(info.hash)

	// Append info.timestamp to data
	copy(data[offset:offset+len(info.timestamp)], info.timestamp[:])
	offset += len(info.timestamp)

	return data, nil
}

func ReceiptToUpdates(receipts []*types.Receipt) ([]*L1UpdateInfo, error) {
	var out []*L1UpdateInfo
	var result error
	for _, rec := range receipts {
		if rec.Status != types.ReceiptStatusSuccessful {
			continue
		}
		for _, rawLog := range rec.Logs {
			if rawLog.Address == L1WorldcoinRegistry && len(rawLog.Topics) > 0 && rawLog.Topics[0] == RootSentEventABIHash {
				log.Info("ATENTIONNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN")
				ev := rawLog
				infoDat := &L1UpdateInfo{
					hash:      common.BytesToHash(ev.Data[0:32]),
					timestamp: common.BytesToHash(ev.Data[32:64]),
				}
				out = append(out, infoDat)
			}
		}
	}
	return out, result
}

// Additionally, the event log-index and
func WorldcoinTxGivenEvent(deposits []*L1UpdateInfo, block eth.BlockInfo, seqNumber uint64) ([]*types.DepositTx, error) {
	log.Info("WorldcoinTxGivenEvent")
	var depositTxs []*types.DepositTx

	for _, info := range deposits {
		log.Info("info.Calldata")
		data, err := info.CallData()
		if err != nil {
			return nil, err
		}

		log.Info("SOURCE")
		source := L1InfoDepositSource{
			L1BlockHash: block.Hash(),
			SeqNumber:   seqNumber,
		}

		log.Info("C3: Worldcoin update calldata: ", string(data), L2RegistryContract.String())
		depositTx := &types.DepositTx{
			SourceHash:          source.SourceHash(),
			From:                L1InfoDepositerAddress,
			To:                  &L2RegistryContract,
			Mint:                nil,
			Value:               big.NewInt(10),
			Gas:                 150_000_000,
			IsSystemTransaction: true,
			Data:                data,
		}

		depositTxs = append(depositTxs, depositTx)
	}

	return depositTxs, nil
}

func L1UpdateWorldcoinBytes(receipts []*types.Receipt, l1Info eth.BlockInfo, seqNumber uint64) ([]hexutil.Bytes, error) {
	var result error
	updates, err := ReceiptToUpdates(receipts)
	log.Info("RAFAEL updates", strconv.Itoa(len(updates)))

	if err != nil {
		result = multierror.Append(result, err)
	}

	updateTxs, err := WorldcoinTxGivenEvent(updates, l1Info, seqNumber)

	if err != nil {
		result = multierror.Append(result, err)
	}

	encodedTxs := make([]hexutil.Bytes, 0, len(updateTxs))
	for i, tx := range updateTxs {
		opaqueTx, err := types.NewTx(tx).MarshalBinary()
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to encode user tx %d", i))
		} else {
			encodedTxs = append(encodedTxs, opaqueTx)
		}
	}
	return encodedTxs, result
}
