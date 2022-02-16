package native

import (
	"encoding/json"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
)

/*"encoding/json"
"errors"
"math/big"
"strconv"
"strings"
"sync/atomic"
"time"

"github.com/ethereum/go-ethereum/common"
"github.com/ethereum/go-ethereum/core/vm"
"github.com/ethereum/go-ethereum/eth/tracers"*/

// EVMLogger is used to collect execution traces from an EVM transaction
// execution. CaptureState is called for each step of the VM with the
// current VM state.
// Note that reference types are actual VM data structures; make copies
// if you need to retain them beyond the current call.
/*type EVMLogger interface {
	CaptureStart(env *EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int)
	CaptureState(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, rData []byte, depth int, err error)
	CaptureEnter(typ OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int)
	CaptureExit(output []byte, gasUsed uint64, err error)
	CaptureFault(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, depth int, err error)
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error)
}
*/

/*

  const std::string gethTracer_ = R"({
    retVal: {calls: [], logs: []},
    valid: true,
    errcode: '',
    result: function(ctx, db) {
      this.retVal.errcode = this.errcode;
      this.retVal.valid = this.valid;
      return this.retVal;
    },
    fault: function(log,db) {this.valid = false; this.errcode = log.getError();},
    step: function(log, db) {
      var error = log.getError();
      if(error) {
        this.errcode = log.error;
        this.valid = false;
        return;
      }
      if(log.op.toNumber() == 0xf1){
        var stack = [];

        for(var i = 0; i < log.stack.length(); i++) {
          stack.push('0x' + log.stack.peek(i).toString(16));
        }

        var offset = parseInt(stack[3], 16);
        var len = parseInt(stack[4], 16);
        if (len >= 4)
          len = 4;
        var cd = log.memory.slice(offset, offset+len);
        var str = '0x';
        for(var elem in cd) {
          str += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
        }
        this.retVal.calls.push({address: stack[1], calldata: str});
      } else if (log.op.toNumber() == 0xa1) {
        var stack = [];

        for(var i = 0; i < log.stack.length(); i++) {
          stack.push('0x' + log.stack.peek(i).toString(16));
        }
        var offset = parseInt(stack[0], 16);
        var len = parseInt(stack[1], 16);
        var cd = log.memory.slice(offset, offset + len);
        var str = '0x';
        for(var elem in cd) {
          str += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
        }
        cd = log.contract.getAddress();
        var addr = '0x';
        for(var elem in cd) {
          addr += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
        }
        this.retVal.logs.push({topic: stack[2], args: str, contractAddress: addr});
      }
    }
  })";

*/

type Call struct {
	Address  string `json:"address"`
	Calldata string `json:"calldata"`
}

type Logarr struct {
	Calls []Call `json:"calls"`
	Logs  []Log  `json:"logs"`
}

type Logret struct {
	ErrCode string  `json:"errcode"`
	Valid   bool    `json:"valid"`
	Retval  *Logarr `json:"retVal"`
}

type Log struct {
	Topic           string `json:"address"`
	Args            string `json:"args"`
	ContractAddress string `json:"contractAddress"`
}

type logTracer struct {
	env       *vm.EVM
	retValue  Logret
	interrupt uint32 // Atomic flag to signal execution interruption
	reason    error  // Textual reason for the interruption
}

func init() {
	register("logTracer", newLogTracer)
}

func (l *logTracer) String() string {
	res, err := json.Marshal(l.retValue)
	if err != nil {
		return ""
	}
	return string(res)
}

// newCallTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func newLogTracer() tracers.Tracer {
	// First callframe contains tx context info
	// and is populated on start and end.
	t := &logTracer{
		retValue: Logret{
			Valid: true,
			Retval: &Logarr{
				Calls: make([]Call, 5),
				Logs:  make([]Log, 5),
			},
		},
	}
	return t
}

func (l *logTracer) GetResult() (json.RawMessage, error) {
	res, err := json.Marshal(l.retValue)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), l.reason
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *logTracer) Stop(err error) {
	t.reason = err
	atomic.StoreUint32(&t.interrupt, 1)
}

func (l *logTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	l.env = env
}

func (l *logTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	switch op {
	case 0xa1:
		{
			/*
				var stack = [];

					for(var i = 0; i < log.stack.length(); i++) {
						stack.push('0x' + log.stack.peek(i).toString(16));
					}
					var offset = parseInt(stack[0], 16);
					var len = parseInt(stack[1], 16);
					var cd = log.memory.slice(offset, offset + len);
					var str = '0x';
					for(var elem in cd) {
						str += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
					}
					cd = log.contract.getAddress();
					var addr = '0x';
					for(var elem in cd) {
						addr += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
					}
					this.retVal.logs.push({topic: stack[2], args: str, contractAddress: addr});
			*/
			stackArr := scope.Stack.Data()
			offset := stackArr[0].Uint64()
			len := stackArr[1].Uint64()
			mem := scope.Memory.Data()
			args := mem[offset : offset+len]
			topic := stackArr[2]
			address := scope.Contract.CodeAddr
			log := Log{
				Topic:           bigToHex(topic.ToBig()),
				Args:            bytesToHex(args),
				ContractAddress: addrToHex(*address),
			}
			l.retValue.Retval.Logs = append(l.retValue.Retval.Logs, log)
		}
	case 0xf1:
		{
			/*
				f(log.op.toNumber() == 0xf1){
				var stack = [];

				for(var i = 0; i < log.stack.length(); i++) {
					stack.push('0x' + log.stack.peek(i).toString(16));
				}

				var offset = parseInt(stack[3], 16);
				var len = parseInt(stack[4], 16);
				if (len >= 4)
					len = 4;
				var cd = log.memory.slice(offset, offset+len);
				var str = '0x';
				for(var elem in cd) {
					str += ('0' + (cd[elem] & 0xFF).toString(16)).slice(-2);
				}
				this.retVal.calls.push({address: stack[1], calldata: str});

			*/
			stackArr := scope.Stack.Data()
			offset := stackArr[3].Uint64()
			len := stackArr[4].Uint64()
			if len >= 4 {
				len = 4
			}
			mem := scope.Memory.Data()
			calldata := mem[offset : offset+len]
			call := Call{
				Address:  bigToHex(stackArr[1].ToBig()),
				Calldata: bytesToHex(calldata),
			}
			l.retValue.Retval.Calls = append(l.retValue.Retval.Calls, call)
		}
	default:
		{
			// do nothing
		}
	}
}

func (l *logTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {

}

func (l *logTracer) CaptureExit(output []byte, gasUsed uint64, err error) {

}

func (l *logTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
	l.retValue.Valid = false
	if err != nil {
		l.retValue.ErrCode = err.Error()
	}
}

func (l *logTracer) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) {
	if err != nil {
		l.retValue.Valid = false
		l.retValue.ErrCode = err.Error()
	}
}
