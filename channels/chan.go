package channels

import (
	"sync"
)

// no generics means code duplication 👍
// trust me i tried to use go's pre-generic generic workaround and it didnt work
const Fail = -0xff

// declare channel structs and interface for each type we need

/* channel implementation for integers */
type IntChannel struct {
	value      *int
	cond       *sync.Cond
}
/* channel implementation for bools*/
type BoolChannel struct {
	value      *bool
	cond       *sync.Cond
}

// define the initialisation functions
func NewIntChannel() *IntChannel {
	m := new(sync.Mutex)
	return &IntChannel{value: nil, cond: sync.NewCond(m)}
}

func NewBoolChannel() *BoolChannel {
	m := sync.Mutex{}
	return &BoolChannel{value: nil, cond: sync.NewCond(&m)}
}

// define implementations

/*
Sends a value into the channel
 - value {int} the value to send
 - block {bool} should we block the thread until sent?
*/
func (c *IntChannel) Send(value int, block bool) {
	c.cond.L.Lock()
	for c.value != nil {
		if !block {
			c.cond.L.Unlock()
			return
		}
		c.cond.Wait()
	}
	c.value = &value
	c.cond.Broadcast()
	c.cond.L.Unlock()
}

/*
Receives a value from the channel
 - block {bool} should we block thread until we get something?
 - returns value {int}, success {bool}
If blocking receiving, then the success value can be ignored
*/
func (c *IntChannel) Receive(block bool) (value int, success bool) {
	c.cond.L.Lock()
	for c.value == nil {
		if !block {
			c.cond.L.Unlock()
			return Fail, false
		}
		c.cond.Wait()
	}
	defer func() {
		c.cond.Broadcast()
		c.cond.L.Unlock()
	}()
	value = *c.value
	c.value = nil
	return value, true
}

/*
Sends a value into the channel
 - value {bool} the value to send
 - block {bool} should we block the thread until sent?
*/
func (c *BoolChannel) Send(value bool, block bool) {
	c.cond.L.Lock()
	for c.value != nil {
		if !block {
			c.cond.L.Unlock()
			return
		}
		c.cond.Wait()
	}
	c.value = &value
	c.cond.Broadcast()
	c.cond.L.Unlock()
}

/*
Receives a value from the channel
 - block {bool} should we block thread until we get something?
 - returns value {bool}, success {bool}
If blocking receiving, then the success value can be ignored
*/
func (c *BoolChannel) Receive(block bool) (bool, success bool) {
	c.cond.L.Lock()
	for c.value == nil {
		if !block {
			c.cond.L.Unlock()
			return false, false
		}
		c.cond.Wait()
	}
	defer func() {
		c.cond.Broadcast()
		c.cond.L.Unlock()
	}()
	value := *c.value
	c.value = nil
	return value, false
}


