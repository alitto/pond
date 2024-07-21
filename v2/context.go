package pond

import (
	"context"
	"fmt"
)

type outputOrErr[O any] struct {
	err    error
	output O
}

func (r *outputOrErr[O]) Err() error {
	return r.err
}

func (r *outputOrErr[O]) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return fmt.Sprintf("result: %#v", r.output)
}

type TaskContext[O any] interface {
	Done() <-chan struct{}
	Err() error
	Output() O
	Wait() (O, error)
	WaitErr() error
}

type taskContext[O any] struct {
	context.Context
}

func (c *taskContext[O]) Output() O {
	cause := context.Cause(c.Context)
	if r, ok := cause.(*outputOrErr[O]); ok {
		return r.output
	}
	var zero O
	return zero
}

func (c *taskContext[O]) Err() error {
	cause := context.Cause(c.Context)
	if r, ok := cause.(*outputOrErr[O]); ok {
		return r.Err()
	}
	return cause
}

func (c *taskContext[O]) Wait() (O, error) {
	<-c.Context.Done()
	return c.Output(), c.Err()
}

func (c *taskContext[O]) WaitErr() error {
	<-c.Context.Done()
	return c.Err()
}
