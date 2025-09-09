package conf

type Evm struct {
	HostName     string `yaml:"hostname" json:"hostname"`
	ChainName    string `yaml:"chain_name" json:"chain_name"`
	ProtocolName string `yaml:"protocol_name" json:"protocol_name"`
	ChainId      string `yaml:"chain_id" json:"chain_id"`
	NodeVersion  string `yaml:"node_version" json:"node_version"`
	HttpURL      string `yaml:"http_url" json:"http_url"`
	WsURL        string `yaml:"ws_url" json:"ws_url"`
	CheckSecond  int    `yaml:"check_second" json:"check_second"`
}

type Cometbft struct {
	HostName     string `yaml:"hostname" json:"hostname"`
	ChainName    string `yaml:"chain_name" json:"chain_name"`
	ProtocolName string `yaml:"protocol_name" json:"protocol_name"`
	ChainId      string `yaml:"chain_id" json:"chain_id"`
	NodeVersion  string `yaml:"node_version" json:"node_version"`
	HttpURL      string `yaml:"http_url" json:"http_url"`
	WsEndpoint   string `yaml:"ws_endpoint" json:"ws_endpoint"`
	CheckSecond  int    `yaml:"check_second" json:"check_second"`
}

type NodeConfig struct {
	Evm      []*Evm      `yaml:"evm" json:"evm"`
	Cometbft []*Cometbft `yaml:"cometbft" json:"cometbft"`
}
