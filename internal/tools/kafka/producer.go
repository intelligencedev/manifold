package kafka

import (
	"fmt"
	"strings"

	"github.com/segmentio/kafka-go"
)

// NewProducerFromBrokers creates a new Kafka producer (Writer) from broker addresses.
func NewProducerFromBrokers(brokers string) (Writer, error) {
	if brokers = strings.TrimSpace(brokers); brokers == "" {
		return nil, fmt.Errorf("kafka brokers cannot be empty")
	}

	brokerList := strings.Split(brokers, ",")
	for i, b := range brokerList {
		brokerList[i] = strings.TrimSpace(b)
	}

	w := &kafka.Writer{
		Addr:     kafka.TCP(brokerList...),
		Balancer: &kafka.LeastBytes{},
	}

	return w, nil
}
