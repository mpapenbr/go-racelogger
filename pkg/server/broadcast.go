package server

import (
	"sync"
)

type Broadcaster[E any] struct {
	mu          sync.Mutex
	subscribers []chan E
	lastValue   *E
}

// creates a new Broadcaster instance
// E is the type of events that will be broadcasted
// New subscribers will receive the last broadcasted value immediately upon subscription
func NewBroadcaster[E any]() *Broadcaster[E] {
	return &Broadcaster[E]{
		subscribers: make([]chan E, 0),
	}
}

func (b *Broadcaster[E]) Subscribe() <-chan E {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan E, 1) // Buffered channel to avoid blocking
	b.subscribers = append(b.subscribers, ch)
	go func() {
		if b.lastValue != nil {
			// If there is a last value, send it immediately to the new subscriber
			// This is useful for subscribers that join after the last broadcast
			ch <- *b.lastValue
		}
	}()
	return ch
}

func (b *Broadcaster[E]) Unsubscribe(ch <-chan E) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, subscriber := range b.subscribers {
		if subscriber == ch {
			close(subscriber) // Close the channel to signal no more messages
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			return
		}
	}
}

func (b *Broadcaster[E]) Broadcast(event E) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastValue = &event // Store the last value to send to new subscribers
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
			// If the channel is full, we skip sending to avoid blocking
			// This is a simple way to handle backpressure
		}
	}
}
