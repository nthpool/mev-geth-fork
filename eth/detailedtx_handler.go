package eth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/ethapi"
)

type DetailedTxHandler struct {
	backend *Ethereum
	txsCh   chan core.NewTxsEvent
	txsSub  event.Subscription
	dtxFeed event.Feed
	scope   event.SubscriptionScope
	tracer  *tracers.API
}

func NewDetailedTxHandler(backend *Ethereum) *DetailedTxHandler {
	dh := &DetailedTxHandler{
		backend: backend,
		txsCh:   make(chan core.NewTxsEvent),
		tracer:  tracers.NewAPI(backend.APIBackend),
	}
	dh.txsSub = backend.txPool.SubscribeNewTxsEvent(dh.txsCh)
	go dh.txLoop()
	return dh
}

func (d *DetailedTxHandler) SubscribeDetailedPendingTxEvent(dc chan<- core.NewDetailedTxsEvent) event.Subscription {
	return d.scope.Track(d.dtxFeed.Subscribe(dc))
}

func (d *DetailedTxHandler) customTrace(ctx context.Context, message core.Message,
	txctx *tracers.Context, vmctx vm.BlockContext, statedb *state.StateDB,
	config *tracers.TraceConfig) (interface{}, error) {
	// Assemble the structured logger or the JavaScript tracer
	var (
		tracer    vm.EVMLogger
		err       error
		txContext = core.NewEVMTxContext(message)
	)
	switch {
	case config == nil:
		tracer = logger.NewStructLogger(nil)
	case config.Tracer != nil:
		// Define a meaningful timeout of a single transaction trace
		timeout := 5 * time.Second
		if config.Timeout != nil {
			if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
				return nil, err
			}
		}
		if t, err := tracers.New(*config.Tracer, txctx); err != nil {
			return nil, err
		} else {
			deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
			go func() {
				<-deadlineCtx.Done()
				if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
					t.Stop(errors.New("execution timeout"))
				}
			}()
			defer cancel()
			tracer = t
		}
	default:
		tracer = logger.NewStructLogger(config.Config)
	}
	// Run the transaction with tracing enabled.
	vmenv := vm.NewEVM(vmctx, txContext, statedb, d.backend.APIBackend.ChainConfig(), vm.Config{Debug: true, Tracer: tracer, NoBaseFee: true})

	// Call Prepare to clear out the statedb access list
	statedb.Prepare(txctx.TxHash, txctx.TxIndex)

	result, err := core.ApplyMessage(vmenv, message, new(core.GasPool).AddGas(message.Gas()))
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
	case *logger.StructLogger:
		// If the result contains a revert reason, return it.
		returnVal := fmt.Sprintf("%x", result.Return())
		if len(result.Revert()) > 0 {
			returnVal = fmt.Sprintf("%x", result.Revert())
		}
		return &ethapi.ExecutionResult{
			Gas:         result.UsedGas,
			Failed:      result.Failed(),
			ReturnValue: returnVal,
			StructLogs:  ethapi.FormatLogs(tracer.StructLogs()),
		}, nil

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
		panic("bad response type")
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

func (d *DetailedTxHandler) txLoop() {
	for {
		select {
		case event := <-d.txsCh:
			dt := d.traceTx(event)
			d.dtxFeed.Send(core.NewDetailedTxsEvent{Txs: dt})
		}
	}

}
