package events

import "sync"

type message struct {
	Kind string
	Data []byte
	prev *message
}

type buffer struct {
	lock sync.Mutex
	head *message
	tail *message
	size int
}

func newBuffer() *buffer {
	return &buffer{}
}

func (b *buffer) PushBack(msg *message) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.head == nil {
		b.head = msg
		b.tail = msg
	} else {
		b.tail.prev = msg
		b.tail = msg
	}
	b.size++

	return nil
}

func (b *buffer) Pop() *message {
	if b.head == nil {
		return nil
	}
	tmp := b.head
	if b.head.prev != nil {
		b.head = b.head.prev
	} else {
		// removing the last one
		b.head = nil
		b.tail = nil
	}
	b.size--
	return tmp
}

func (b *buffer) Size() int {
	return b.size
}
