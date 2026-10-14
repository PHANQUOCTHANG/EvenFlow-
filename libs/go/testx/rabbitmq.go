package testx

import (
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

var rmq started

// RabbitMQ khoi dong (mot lan cho ca tien trinh) rabbitmq:3.13-management-alpine va tra ve URL AMQP.
func RabbitMQ(t testing.TB) string {
	t.Helper()
	return rmq.get(t, RabbitMQImage, startRabbitMQ)
}

func startRabbitMQ() (string, error) {
	ctx, cancel := startCtx()
	defer cancel()

	ctr, err := rabbitmq.Run(ctx, RabbitMQImage)
	if ctr != nil {
		terminate(ctr)
	}
	if err != nil {
		return "", err
	}

	return ctr.AmqpURL(ctx)
}
