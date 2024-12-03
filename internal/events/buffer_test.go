package events

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("buffer", Ordered, func() {
	Context("buffer", func() {
		It("add successfully", func() {
			buffer := newBuffer()

			// add the first message
			err := buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg1")})
			Expect(err).To(BeNil())
			Expect(buffer.Size()).To(Equal(1))
			Expect(buffer.head).NotTo(BeNil())
			Expect(buffer.tail).NotTo(BeNil())

			// second
			err = buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg2")})
			Expect(err).To(BeNil())
			Expect(buffer.Size()).To(Equal(2))
			Expect(buffer.head).NotTo(BeNil())
			Expect(buffer.tail).NotTo(BeNil())

			Expect(buffer.head.Data).To(Equal([]byte("msg1")))
			Expect(buffer.tail.Data).To(Equal([]byte("msg2")))

			// third
			err = buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg3")})
			Expect(err).To(BeNil())
			Expect(buffer.Size()).To(Equal(3))
			Expect(buffer.head).NotTo(BeNil())
			Expect(buffer.tail).NotTo(BeNil())

			Expect(buffer.head.Data).To(Equal([]byte("msg1")))
			Expect(buffer.tail.Data).To(Equal([]byte("msg3")))
		})

		It("pop", func() {
			buffer := newBuffer()

			// add the first message
			err := buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg1")})
			Expect(err).To(BeNil())
			err = buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg2")})
			Expect(err).To(BeNil())
			err = buffer.PushBack(&message{Kind: InventoryMessageKind, Data: []byte("msg3")})
			Expect(err).To(BeNil())
			Expect(buffer.Size()).To(Equal(3))

			m := buffer.Pop()
			Expect(m).NotTo(BeNil())
			Expect(m.Data).To(Equal([]byte("msg1")))
			Expect(buffer.Size()).To(Equal(2))

			m = buffer.Pop()
			Expect(m).NotTo(BeNil())
			Expect(m.Data).To(Equal([]byte("msg2")))
			Expect(buffer.Size()).To(Equal(1))

			m = buffer.Pop()
			Expect(m).NotTo(BeNil())
			Expect(m.Data).To(Equal([]byte("msg3")))
			Expect(buffer.Size()).To(Equal(0))
			Expect(buffer.head).To(BeNil())
			Expect(buffer.tail).To(BeNil())

			m = buffer.Pop()
			Expect(m).To(BeNil())
		})
	})
})
