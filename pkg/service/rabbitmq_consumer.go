package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
)

const (
	rabbitMQReconnectDelay = 5 * time.Second
	rabbitMQConsumeTimeout = 30 * time.Second
	rabbitMQPublishTimeout = 5 * time.Second
	rabbitMQHeaderRetry    = "x-retry-count"
	rabbitMQHeaderError    = "x-last-error"
	rabbitMQKindTx         = "tx"
	rabbitMQKindRollback   = "rollback"
)

type callbackConsumer struct {
	cfg *config.RabbitMQ
	svc WalletService
}

type callbackQueueSpec struct {
	Kind            string
	Exchange        string
	MainQueue       string
	RetryQueue      string
	DeadLetterQueue string
}

func shouldUseRabbitMQ(cfg *config.RabbitMQ) bool {
	return cfg != nil && (cfg.Enabled || strings.TrimSpace(cfg.URL) != "")
}

func newCallbackConsumer(cfg *config.RabbitMQ, svc WalletService) *callbackConsumer {
	return &callbackConsumer{cfg: cfg, svc: svc}
}

func (c *callbackConsumer) Start() error {
	for _, spec := range c.queueSpecs() {
		if err := c.verifyTopology(spec); err != nil {
			return err
		}
	}
	for _, spec := range c.queueSpecs() {
		go c.consumeLoop(spec)
	}
	return nil
}

func (c *callbackConsumer) queueSpecs() []callbackQueueSpec {
	return []callbackQueueSpec{
		{
			Kind:            rabbitMQKindTx,
			Exchange:        c.cfg.TxExchange,
			MainQueue:       c.cfg.TxQueue,
			RetryQueue:      c.cfg.TxQueue + ".delay",
			DeadLetterQueue: c.cfg.TxQueue + ".dlq",
		},
		{
			Kind:            rabbitMQKindRollback,
			Exchange:        c.cfg.RollbackExchange,
			MainQueue:       c.cfg.RollbackQueue,
			RetryQueue:      c.cfg.RollbackQueue + ".delay",
			DeadLetterQueue: c.cfg.RollbackQueue + ".dlq",
		},
	}
}

func (c *callbackConsumer) verifyTopology(spec callbackQueueSpec) error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	return c.declareTopology(ch, spec)
}

func (c *callbackConsumer) consumeLoop(spec callbackQueueSpec) {
	for {
		if err := c.consume(spec); err != nil {
			log.Warningf("rabbitmq consumer stopped kind=%s queue=%s err=%v", spec.Kind, spec.MainQueue, err)
		}
		time.Sleep(rabbitMQReconnectDelay)
	}
}

func (c *callbackConsumer) consume(spec callbackQueueSpec) error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// 限制单个 consumer 同时拿到多少个未确认消息。这里避免消息无限堆到单个实例内存里，也让手动 ack 更可控
	if err := ch.Qos(c.cfg.PrefetchCount, 0, false); err != nil {
		return err
	}
	if err := c.declareTopology(ch, spec); err != nil {
		return err
	}

	deliveries, err := ch.Consume(spec.MainQueue, "projectc-custodial-wallet-"+spec.Kind, false, false, false, false, nil)
	if err != nil {
		return err
	}

	log.Infof("rabbitmq consumer started kind=%s queue=%s", spec.Kind, spec.MainQueue)
	for delivery := range deliveries {
		if err := c.handleDelivery(spec, delivery); err != nil {
			log.Warningf("rabbitmq handle delivery failed kind=%s queue=%s err=%v", spec.Kind, spec.MainQueue, err)
		}
	}
	return fmt.Errorf("rabbitmq deliveries closed kind=%s queue=%s", spec.Kind, spec.MainQueue)
}

func (c *callbackConsumer) declareTopology(ch *amqp.Channel, spec callbackQueueSpec) error {
	if err := ch.ExchangeDeclare(spec.Exchange, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(spec.MainQueue, true, false, false, false, nil); err != nil {
		return err
	}

	//- RetryQueue 是延迟队列
	//- 进入这个队列的消息不会立刻被消费
	//- 它会先在队列里待 retryDelayMs
	//- 到了 TTL 之后，RabbitMQ 会把它 dead-letter 到 spec.Exchange
	//- 然后因为主队列已经绑定在这个 exchange 上
	//- 所以消息会重新回到 MainQueue
	if _, err := ch.QueueDeclare(spec.RetryQueue, true, false, false, false, amqp.Table{
		"x-message-ttl":          int32(c.cfg.RetryDelayMs),
		"x-dead-letter-exchange": spec.Exchange,
	}); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(spec.DeadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return ch.QueueBind(spec.MainQueue, "", spec.Exchange, false, nil)
}

func (c *callbackConsumer) handleDelivery(spec callbackQueueSpec, delivery amqp.Delivery) error {
	ctx, cancel := context.WithTimeout(context.Background(), rabbitMQConsumeTimeout)
	defer cancel()

	var err error
	switch spec.Kind {
	case rabbitMQKindTx:
		var msg txCallbackMessage
		if unmarshalErr := json.Unmarshal(delivery.Body, &msg); unmarshalErr != nil {
			err = fmt.Errorf("unmarshal tx callback: %w", unmarshalErr)
			break
		}
		err = c.svc.HandleTxCallback(ctx, models.ConnectorTxCallbackRequest(msg))
	case rabbitMQKindRollback:
		var msg txRollbackMessage
		if unmarshalErr := json.Unmarshal(delivery.Body, &msg); unmarshalErr != nil {
			err = fmt.Errorf("unmarshal rollback callback: %w", unmarshalErr)
			break
		}
		err = c.svc.HandleRollbackCallback(ctx, models.ConnectorTxRollbackRequest(msg))
	default:
		err = fmt.Errorf("unsupported callback kind=%s", spec.Kind)
	}

	if err == nil {
		return delivery.Ack(false)
	}

	return c.requeueOrDeadLetter(spec, delivery, err)
}

// 这里的延迟实现不是 RabbitMQ 插件版 delayed exchange，而是 TTL + DLX 的经典方案：
//
// 1. 失败消息先发布到 RetryQueue
// 2. 在 RetryQueue 里等 retryDelayMs
// 3. 到期后自动 dead-letter 回原来的 Exchange
// 4. Exchange 再把消息投递回 MainQueue
// 5. consumer 再次消费

// 1. 取出当前重试次数 retryCount(headers) + 1
// 2. 如果没超过 MaxRetry
// - 发布到 RetryQueue
// 3. 如果超过 MaxRetry
// - 发布到 DeadLetterQueue
// 4. 发布成功后，Ack 原消息
func (c *callbackConsumer) requeueOrDeadLetter(spec callbackQueueSpec, delivery amqp.Delivery, cause error) error {
	nextRetry := retryCount(delivery.Headers) + 1
	targetQueue := spec.RetryQueue
	if nextRetry > c.cfg.MaxRetry {
		targetQueue = spec.DeadLetterQueue
	}

	//1. 先尝试把 fallback 消息安全发出去
	//2. 如果发成功，再 Ack 原消息
	//3. 如果发失败，就不要 ack 原消息，而是 Nack(requeue=true) 让 RabbitMQ 把这条原消息重新放回队列，后面还能再被消费一次
	if err := c.publishToQueue(targetQueue, delivery, nextRetry, cause); err != nil {
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			return fmt.Errorf("publish fallback failed: %v; nack failed: %w", err, nackErr)
		}
		return err
	}

	if ackErr := delivery.Ack(false); ackErr != nil {
		return ackErr
	}

	log.Warningf("rabbitmq callback rerouted kind=%s txRetry=%d targetQueue=%s err=%v", spec.Kind, nextRetry, targetQueue, cause)
	return nil
}

func (c *callbackConsumer) publishToQueue(queue string, delivery amqp.Delivery, retry int, cause error) error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		return err
	}

	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	// 复制并增强 headers
	headers := cloneHeaders(delivery.Headers)
	headers[rabbitMQHeaderRetry] = int32(retry)
	headers[rabbitMQHeaderError] = truncateString(cause.Error(), 512)

	ctx, cancel := context.WithTimeout(context.Background(), rabbitMQPublishTimeout)
	defer cancel()

	// 用 RabbitMQ 默认 exchange，把 routing key 当作队列名，类型可以理解成一种内建的 direct 行为，队列在创建时，都会自动和这个默认交换机建立一条绑定关系，只要队列存在，且 routing key 等于队列名，就能投进去
	if err := ch.PublishWithContext(ctx, "", queue, true, false, amqp.Publishing{
		Headers:         headers,
		ContentType:     delivery.ContentType,
		ContentEncoding: delivery.ContentEncoding,
		Body:            delivery.Body,
		DeliveryMode:    amqp.Persistent,
		Timestamp:       time.Now(),
		Type:            delivery.Type,
	}); err != nil {
		return err
	}

	// 监听 Return 和 Confirm
	select {
	case returned := <-returns:
		return fmt.Errorf("rabbitmq fallback returned queue=%s replyCode=%d replyText=%s", queue, returned.ReplyCode, returned.ReplyText)
	case confirm := <-confirms:
		if !confirm.Ack {
			return fmt.Errorf("rabbitmq fallback publish nack queue=%s deliveryTag=%d", queue, confirm.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("rabbitmq fallback publish timeout queue=%s: %w", queue, ctx.Err())
	}
}

func retryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch value := headers[rabbitMQHeaderRetry].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	default:
		return 0
	}
}

func cloneHeaders(headers amqp.Table) amqp.Table {
	if len(headers) == 0 {
		return amqp.Table{}
	}
	cloned := make(amqp.Table, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
