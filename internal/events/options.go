package events

type ProducerOptions func(e *EventProducer)

func WithOutputTopic(topic string) ProducerOptions {
	return func(e *EventProducer) {
		e.topic = topic
	}
}
