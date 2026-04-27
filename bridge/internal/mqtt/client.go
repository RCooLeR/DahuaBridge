package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/metrics"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
)

type Client interface {
	Connect(context.Context) error
	Publish(context.Context, string, byte, bool, []byte) error
	PublishJSON(context.Context, string, byte, bool, any) error
	Subscribe(context.Context, string, byte, func(string, []byte)) error
	Close()
}

type PahoClient struct {
	client         mqtt.Client
	cfg            config.MQTTConfig
	logger         zerolog.Logger
	metrics        *metrics.Registry
	availability   string
	publishTimeout time.Duration
}

type NoopClient struct{}

func New(cfg config.MQTTConfig, logger zerolog.Logger, metricsRegistry *metrics.Registry) Client {
	if !cfg.Enabled {
		return NoopClient{}
	}

	l := logger.With().Str("component", "mqtt").Logger()
	availability := fmt.Sprintf("%s/bridge/status", cfg.TopicPrefix)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetKeepAlive(cfg.KeepAlive)
	opts.SetConnectTimeout(cfg.ConnectTimeout)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(cfg.CleanSession)
	opts.SetOrderMatters(false)
	opts.SetOnConnectHandler(func(_ mqtt.Client) {
		l.Info().Str("broker", cfg.Broker).Msg("connected to mqtt broker")
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		l.Error().Err(err).Msg("mqtt connection lost")
	})
	opts.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
		l.Warn().Msg("reconnecting to mqtt broker")
	})
	opts.SetWill(availability, "offline", cfg.QoS, true)

	return &PahoClient{
		client:         mqtt.NewClient(opts),
		cfg:            cfg,
		logger:         l,
		metrics:        metricsRegistry,
		availability:   availability,
		publishTimeout: cfg.PublishTimeout,
	}
}

func (c *PahoClient) Connect(_ context.Context) error {
	token := c.client.Connect()
	if !token.WaitTimeout(c.cfg.ConnectTimeout) {
		return fmt.Errorf("mqtt connect timeout after %s", c.cfg.ConnectTimeout)
	}

	if err := token.Error(); err != nil {
		return err
	}

	return c.Publish(context.Background(), c.availability, c.cfg.QoS, true, []byte("online"))
}

func (c *PahoClient) Publish(ctx context.Context, topic string, qos byte, retain bool, payload []byte) error {
	if !c.client.IsConnected() {
		err := errors.New("mqtt client is not connected")
		c.metrics.ObserveMQTTPublish(topic, err)
		return err
	}

	token := c.client.Publish(topic, qos, retain, payload)
	timeout := c.publishTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	if !token.WaitTimeout(timeout) {
		err := fmt.Errorf("mqtt publish timeout after %s", timeout)
		c.metrics.ObserveMQTTPublish(topic, err)
		return err
	}

	err := token.Error()
	c.metrics.ObserveMQTTPublish(topic, err)
	return err
}

func (c *PahoClient) PublishJSON(ctx context.Context, topic string, qos byte, retain bool, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return c.Publish(ctx, topic, qos, retain, data)
}

func (c *PahoClient) Subscribe(ctx context.Context, topic string, qos byte, handler func(string, []byte)) error {
	if !c.client.IsConnected() {
		return errors.New("mqtt client is not connected")
	}

	token := c.client.Subscribe(topic, qos, func(_ mqtt.Client, message mqtt.Message) {
		if handler != nil {
			handler(message.Topic(), message.Payload())
		}
	})

	timeout := c.publishTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	if !token.WaitTimeout(timeout) {
		return fmt.Errorf("mqtt subscribe timeout after %s", timeout)
	}

	return token.Error()
}

func (c *PahoClient) Close() {
	if !c.client.IsConnected() {
		return
	}

	_ = c.Publish(context.Background(), c.availability, c.cfg.QoS, true, []byte("offline"))
	c.client.Disconnect(250)
}

func (NoopClient) Connect(context.Context) error                              { return nil }
func (NoopClient) Publish(context.Context, string, byte, bool, []byte) error  { return nil }
func (NoopClient) PublishJSON(context.Context, string, byte, bool, any) error { return nil }
func (NoopClient) Subscribe(context.Context, string, byte, func(string, []byte)) error {
	return nil
}
func (NoopClient) Close() {}
