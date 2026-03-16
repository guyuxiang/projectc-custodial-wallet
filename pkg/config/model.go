package config

import "fmt"

type Config struct {
	Server    *Server    `yaml:"server"`
	Auth      *Auth      `yaml:"auth"`
	Gin       *Gin       `yaml:"gin"`
	Log       *Log       `yaml:"log"`
	MySQL     *MySQL     `yaml:"mysql"`
	RabbitMQ  *RabbitMQ  `yaml:"rabbitmq"`
	Signature *Signature `yaml:"signature"`
	KMS       *KMS       `yaml:"kms"`
	Connector *Connector `yaml:"connector"`
	Callback  *Callback  `yaml:"callback"`
	Solana    *Solana    `yaml:"solana"`
}

type Server struct {
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
	Version string `yaml:"version"`
}

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Gin struct {
	Mode string `yaml:"mode"`
}

type Log struct {
	Level string `yaml:"level"`
}

type MySQL struct {
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Database     string `yaml:"database"`
	DSN          string `yaml:"dsn"`
	MaxIdleConns int    `yaml:"maxIdleConns"`
	MaxOpenConns int    `yaml:"maxOpenConns"`
}

func (m *MySQL) EffectiveDSN() string {
	if m == nil {
		return ""
	}
	if m.DSN != "" {
		return m.DSN
	}
	if m.Username == "" || m.Host == "" || m.Port <= 0 || m.Database == "" {
		return ""
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.Username,
		m.Password,
		m.Host,
		m.Port,
		m.Database,
	)
}

type RabbitMQ struct {
	URL            string `yaml:"url"`
	VHost          string `yaml:"vhost"`
	Exchange       string `yaml:"exchange"`
	CancelExchange string `yaml:"cancelExchange"`
	ExchangeType   string `yaml:"exchangeType"`
	Queue          string `yaml:"queue"`
	RoutingKey     string `yaml:"routingKey"`
}

type Signature struct {
	APIKey        string `yaml:"apiKey"`
	PublicKey     string `yaml:"publicKey"`
	PrivateKey    string `yaml:"privateKey"`
	MaxSkewMillis int64  `yaml:"maxSkewMillis"`
}

type KMS struct {
	BaseURL  string `yaml:"baseUrl"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Connector struct {
	BaseURL           string `yaml:"baseUrl"`
	Username          string `yaml:"username"`
	Password          string `yaml:"password"`
	NetworkCode       string `yaml:"networkCode"`
	NativeTokenSymbol string `yaml:"nativeTokenSymbol"`
}

type Callback struct {
	DepositURL     string `yaml:"depositUrl"`
	TransferOutURL string `yaml:"transferOutUrl"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
}

type Solana struct {
	RPCEndpoint      string `yaml:"rpcEndpoint"`
	ComputeUnitPrice uint64 `yaml:"computeUnitPrice"`
}
