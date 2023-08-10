package derive

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

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
	L2RegistryContract = common.HexToAddress("0x045154988bF29b95b2197092EF7A20F3BFeDB94D")
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
	log.Info("ATTENTIONNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNJJ")
	var out []*L1UpdateInfo
	var result error
	for _, rec := range receipts {
		if rec.Status != types.ReceiptStatusSuccessful {
			continue
		}
		for _, rawLog := range rec.Logs {
			if strings.ToLower(rawLog.Address.String()) == strings.ToLower(L1WorldcoinRegistry.String()) {
				log.Info("RAFAEL MATCHINGggggg LOG", rawLog.Topics[0].String(), RootSentEventABIHash.String())
				log.Info("CONDITIONS", strconv.FormatBool(rawLog.Address == L1WorldcoinRegistry), strconv.FormatBool(len(rawLog.Topics) > 0), strconv.FormatBool(rawLog.Topics[0] == RootSentEventABIHash))
			}

			if rawLog.Address == L1WorldcoinRegistry && len(rawLog.Topics) > 0 && rawLog.Topics[0] == RootSentEventABIHash {
				ev := rawLog
				log.Info("AAA")
				if len(ev.Topics) != 2 {
					return nil, fmt.Errorf("expected 4 event topics (event identity, indexed from, indexed to, indexed version), got %d", len(ev.Topics))
				}
				log.Info("BBB")
				if ev.Topics[0] != RootSentEventABIHash {
					return nil, fmt.Errorf("invalid deposit event selector: %s, expected %s", ev.Topics[0], RootSentEventABIHash)
				}
				log.Info("CCC")
				if len(ev.Data) < 64 {
					return nil, fmt.Errorf("incomplate opaqueData slice header (%d bytes): %x", len(ev.Data), ev.Data)
				}
				log.Info("DDD")
				if len(ev.Data)%32 != 0 {
					return nil, fmt.Errorf("expected log data to be multiple of 32 bytes: got %d bytes", len(ev.Data))
				}

				// indexed 0
				infoDat := &L1UpdateInfo{
					hash:      common.BytesToHash(ev.Topics[1][12:]),
					timestamp: common.BytesToHash(ev.Topics[2][12:]),
				}
				out = append(out, infoDat)

			}
		}
	}
	return out, result
}

// Additionally, the event log-index and
func WorldcoinTxGivenEvent(deposits []*L1UpdateInfo, block eth.BlockInfo, seqNumber uint64) ([]*types.DepositTx, error) {
	var depositTxs []*types.DepositTx

	for _, info := range deposits {
		data, err := info.CallData()
		if err != nil {
			return nil, err
		}

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
	log.Info("RAFAEL receipts", strconv.Itoa(len(receipts)))
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
