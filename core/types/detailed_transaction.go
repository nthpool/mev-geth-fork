package types

type TxCall struct {
	Address  string `json:"address"`
	Calldata string `json:"calldata"`
}

type TxLog struct {
	Topic           string `json:"topic"`
	Args            string `json:"args"`
	ContractAddress string `json:"contractAddress"`
}

type Logret struct {
	ErrCode string   `json:"errcode"`
	Valid   bool     `json:"valid"`
	Calls   []TxCall `json:"calls"`
	Logs    []TxLog  `json:"logs"`
}

type DetailedTransaction struct {
	Inner           *Transaction
	ExecutionResult Logret
}
