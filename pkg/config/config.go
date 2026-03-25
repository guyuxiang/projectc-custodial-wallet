package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type flagOpt struct {
	optName         string
	optDefaultValue interface{}
	optUsage        string
}

var c Config

func init() {

	// flag priority: cli > envvars > config > defaults
	for _, opt := range flagsOpts {
		viper.SetDefault(opt.optName, opt.optDefaultValue)
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName(SERVICE_NAME)
	viper.AddConfigPath(fmt.Sprintf("/etc/%s/", SERVICE_NAME))   // path to look for the config file in
	viper.AddConfigPath(fmt.Sprintf("$HOME/.%s/", SERVICE_NAME)) // call multiple times to add many search paths
	viper.AddConfigPath("./etc/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		fmt.Fprintf(os.Stderr, "Fatal error config file: %s \n", err)
	}

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	_ = viper.BindEnv(FLAG_KEY_SOL_RPC, "SOLANA_RPC_ENDPOINT", "SOLANA_RPCENDPOINT")

	for _, opt := range flagsOpts {
		switch opt.optDefaultValue.(type) {
		case int:
			pflag.Int(opt.optName, opt.optDefaultValue.(int), opt.optUsage)
		case int64:
			pflag.Int64(opt.optName, opt.optDefaultValue.(int64), opt.optUsage)
		case uint64:
			pflag.Uint64(opt.optName, opt.optDefaultValue.(uint64), opt.optUsage)
		case string:
			pflag.String(opt.optName, opt.optDefaultValue.(string), opt.optUsage)
		case bool:
			pflag.Bool(opt.optName, opt.optDefaultValue.(bool), opt.optUsage)
		case []interface{}:
			continue
		default:
			continue
		}
	}

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	err = viper.Unmarshal(&c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to decode into struct, %v\n", err)
	}
	normalizeConfig(&c)
}

func GetString(key string) string {
	return viper.GetString(key)
}

func GetInt(key string) int {
	return viper.GetInt(key)
}

func GetBool(key string) bool {
	return viper.GetBool(key)
}

func GetConfig() *Config {
	return &c
}

func normalizeConfig(c *Config) {
	if c.Server == nil {
		c.Server = &Server{}
	}
	if c.Auth == nil {
		c.Auth = &Auth{}
	}
	if c.Gin == nil {
		c.Gin = &Gin{}
	}
	if c.Log == nil {
		c.Log = &Log{}
	}
	if c.MySQL == nil {
		c.MySQL = &MySQL{}
	}
	if c.KMS == nil {
		c.KMS = &KMS{}
	}
	if c.Callback == nil {
		c.Callback = &Callback{}
	}
	if c.RabbitMQ == nil {
		c.RabbitMQ = &RabbitMQ{}
	}
	if c.RequestSignature == nil {
		c.RequestSignature = &RequestSignature{}
	}
	if c.Connectors == nil {
		c.Connectors = make(map[string]*Connector)
	}
	if c.RabbitMQ.TxExchange == "" {
		c.RabbitMQ.TxExchange = "tx_callback_fanout_exchange"
	}
	if c.RabbitMQ.RollbackExchange == "" {
		c.RabbitMQ.RollbackExchange = "tx_callback_cancel_fanout_exchange"
	}
	if c.RabbitMQ.TxQueue == "" {
		c.RabbitMQ.TxQueue = "projectc.wallet.callback.tx"
	}
	if c.RabbitMQ.RollbackQueue == "" {
		c.RabbitMQ.RollbackQueue = "projectc.wallet.callback.rollback"
	}
	if c.RabbitMQ.RetryDelayMs <= 0 {
		c.RabbitMQ.RetryDelayMs = 30000
	}
	if c.RabbitMQ.MaxRetry <= 0 {
		c.RabbitMQ.MaxRetry = 5
	}
	if c.RabbitMQ.PrefetchCount <= 0 {
		c.RabbitMQ.PrefetchCount = 10
	}
}
