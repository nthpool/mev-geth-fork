package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/eth/tracers/native"
	"github.com/ethereum/go-ethereum/event"
)

type DetailedTxHandler struct {
	backend  *Ethereum
	txsCh    chan core.NewTxsEvent
	chainCh  chan core.ChainEvent // Channel to receive new chain event
	txsSub   event.Subscription
	chainSub event.Subscription
	dtxFeed  event.Feed
	scope    event.SubscriptionScope
	hDtxFeed event.Feed
	hScope   event.SubscriptionScope
}

func NewDetailedTxHandler(backend *Ethereum) *DetailedTxHandler {
	dh := &DetailedTxHandler{
		backend: backend,
		txsCh:   make(chan core.NewTxsEvent),
		chainCh: make(chan core.ChainEvent),
	}
	dh.txsSub = backend.txPool.SubscribeNewTxsEvent(dh.txsCh)
	dh.chainSub = backend.APIBackend.SubscribeChainEvent(dh.chainCh)
	go dh.txLoop()
	return dh
}

func (d *DetailedTxHandler) SubscribeDetailedPendingTxEvent(dc chan<- core.NewDetailedTxsEvent) event.Subscription {
	return d.scope.Track(d.dtxFeed.Subscribe(dc))
}

func (d *DetailedTxHandler) SubscribeHeadDetailedPendingTxEvent(dc chan<- core.NewDetailedTxsEvent) event.Subscription {
	return d.hScope.Track(d.hDtxFeed.Subscribe(dc))
}

func (d *DetailedTxHandler) customTrace(ctx context.Context, message core.Message,
	txctx *tracers.Context, vmctx vm.BlockContext, statedb *state.StateDB,
	config *tracers.TraceConfig) (interface{}, error) {
	// Assemble the structured logger or the JavaScript tracer
	var (
		err       error
		txContext = core.NewEVMTxContext(message)
	)

	tracer := native.NewLogTracer()

	// Run the transaction with tracing enabled.
	vmenv := vm.NewEVM(vmctx, txContext, statedb, d.backend.APIBackend.ChainConfig(), vm.Config{Debug: true, Tracer: tracer, NoBaseFee: true})

	// Call Prepare to clear out the statedb access list
	statedb.Prepare(txctx.TxHash, txctx.TxIndex)

	_, err = core.ApplyMessage(vmenv, message, new(core.GasPool).AddGas(message.Gas()))
	if err != nil {
		switch tracer := tracer.(type) {
		case tracers.Tracer:
			// call GetResult to clean up the JS vm
			tracer.GetResult()
		default:
			// do nothing
			break
		}
		return nil, fmt.Errorf("tracing failed: %w", err)
	}

	// Depending on the tracer type, format and return the output.
	switch tracer := tracer.(type) {
	case tracers.Tracer:
		return tracer.GetResult()

	default:
		panic(fmt.Sprintf("bad tracer type %T", tracer))
	}
}

func (d *DetailedTxHandler) makeDetailedTx(tx *types.Transaction) (*types.DetailedTransaction, error) {
	config := d.backend.BlockChain().Config()
	//blockNumber := rpc.PendingBlockNumber.Int64()
	block := d.backend.BlockChain().CurrentBlock()
	signer := types.MakeSigner(config, big.NewInt(0).SetUint64(block.NumberU64()))
	msg, err := tx.AsMessage(signer, block.BaseFee())
	if err != nil {
		return nil, err
	}
	statedb, err := d.backend.StateAtBlock(block, 128, nil, true, false)
	if err != nil {
		return nil, err
	}
	vmctx := core.NewEVMBlockContext(block.Header(), d.backend.BlockChain(), nil)
	res, err := d.customTrace(context.TODO(), msg, new(tracers.Context), vmctx, statedb, nil)
	if err != nil {
		return nil, err
	}
	jsond, ok := res.(json.RawMessage)
	if !ok {
		fmt.Fprintf(os.Stderr, "DetailedTxHandler customTrace bad response type\n") //panic("bad response type")
		return nil, err
	}
	ex := types.Logret{}
	err = json.Unmarshal(jsond, &ex)
	if err != nil {
		return nil, err
	}
	return &types.DetailedTransaction{Inner: tx, ExecutionResult: ex}, nil
}

func (d *DetailedTxHandler) traceTx(event core.NewTxsEvent) []*types.DetailedTransaction {
	dtxa := make([]*types.DetailedTransaction, 0, len(event.Txs))
	for _, tx := range event.Txs {
		dtx, err := d.makeDetailedTx(tx)
		if err != nil {
			continue
		}
		dtxa = append(dtxa, dtx)
	}
	return dtxa
}

func (d *DetailedTxHandler) iteratePendingTxs() {
	fmt.Println("iteratePendingTxs()")
	pending := d.backend.txPool.Pending(true)
	dtxa := make([]*types.DetailedTransaction, 0, len(pending))
	for _, txs := range pending {
		dtx := d.traceTx(core.NewTxsEvent{Txs: txs})
		for _, tx := range dtx {
			if len(tx.ExecutionResult.Logs) > 0 {
				dtxa = append(dtxa, tx)
			}
		}
	}
	fmt.Println("sending ", len(dtxa), " pending txs")
	d.hDtxFeed.Send(core.NewDetailedTxsEvent{Txs: dtxa})
}

func (d *DetailedTxHandler) txLoop() {
	for {
		select {
		case event := <-d.txsCh:
			dt := d.traceTx(event)
			dtxa := make([]*types.DetailedTransaction, 0, len(dt))
			for _, dtx := range dt {
				if len(dtx.ExecutionResult.Logs) > 0 {
					dtxa = append(dtxa, dtx)
				}
			}
			d.dtxFeed.Send(core.NewDetailedTxsEvent{Txs: dtxa})
		case <-d.chainCh:
			go d.iteratePendingTxs()
		}
	}

}
