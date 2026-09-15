package utils

type Deque[T any] struct {
	buffer []T
	size int
	capacity int
	head int
}

func (dq *Deque[T]) getRear() int {
	rear := (dq.head + dq.size) % dq.capacity
	return rear
}

func (dq *Deque[T]) Full() bool {
	return dq.size == dq.capacity
}

func (dq *Deque[T]) PushBack(value T) {
	if dq.Full() {
		panic("Can't push back on an full deque")
	}
	rear := dq.getRear()
	dq.buffer[rear] = value
	dq.size++ 
}

func (dq *Deque[T]) Front() T {
	return dq.buffer[dq.head]
}

func (dq *Deque[T]) Len() int {
	return dq.size
}

func (dq *Deque[T]) Empty() bool {
	return dq.size == 0
}

func (dq *Deque[T]) Pophead() T {
    if dq.size == 0 {
		panic("Can't pop an empty deque")
	}

	value := dq.buffer[dq.head]
	dq.head = (dq.head + 1) % dq.capacity
	dq.size--
	return value
}

func NewDeque[T any](dq Deque[T]) *Deque[T] {
	if dq.capacity <= 0 {
		panic("Deque must have positive capacity.")
	}
	return &Deque[T] {
		capacity: dq.capacity,
		size: 0,
		head: 0,
		buffer: make([]T, dq.capacity),
	}
}