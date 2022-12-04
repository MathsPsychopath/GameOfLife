package channels

import (
	"sync"
)

// no generics means weird go workarounds 👍

// homemade channel implementation
type Channel struct {
	value      *interface{}
	cond       *sync.Cond
}

type ChanInterface interface {
	Send(value interface{}, block bool)
	Receive(block bool) interface{}
}

// define the initialisation function
func NewChannel() *Channel {
	m := sync.Mutex{}
	return &Channel{value: nil, cond: sync.NewCond(&m)}
}

// define interface method implementations

/*
Send a value down the channel
 - value {interface{}} value to send 
 - block {bool} should we block until the value has been successfully sent?
*/
func (c *Channel) Send(value interface{}, block bool) {
	c.cond.L.Lock()
	for c.value != nil {
		if !block {
			return
		}
		c.cond.Wait()
	}
	*c.value = value
	c.cond.Broadcast()
}

/*
Receive a value from the channel
 - block {bool} should we block until a value has been retrieved?
 - returns value {interface{}} - requires type assertion before use
*/
func (c *Channel) Receive(block bool) interface{} {
	c.cond.L.Lock()
	for c.value == nil {
		if !block {
			return nil
		}
		c.cond.Wait()
	}
	defer c.cond.Broadcast()
	value := *c.value
	c.value = nil
	return value
}

/*
helper function for checking. Use when doing type assertion
 - ok {bool} can the underlying type of {interface{}} be casted?
*/
func CheckOk(ok bool) {
	if !ok {
		panic("Incorrect type was received")
	}
}